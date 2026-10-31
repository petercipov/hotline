package proxy

import (
	"crypto/tls"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"

	"hotline-proxy/internal/export"
	"hotline-proxy/internal/spec"
)

// finish builds the call's record and trace once the reply has gone back to
// the app, and hands them to the exporters without waiting for them.
func (c *call) finish() {
	end := c.h.clock.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	attrs := []attribute.KeyValue{
		attribute.String("hotline.integration.id", orNotPassed(c.id)),
		attribute.String("hotline.ingress", c.ingress),
		semconv.ServerAddress(c.host),
		semconv.HTTPRequestMethodKey.String(c.in.Method),
		semconv.HTTPResponseStatusCode(c.status),
	}
	if c.respBody != nil {
		if _, err := c.respBody.counts(); err != nil {
			c.errorType, c.failedStep = "http_response_incomplete", "receive reply"
		}
	}
	if c.errorType != "" {
		attrs = append(attrs, semconv.ErrorTypeKey.String(c.errorType))
	}
	if c.peer != "" {
		// The IP the proxy connected to, after DNS and any try at a
		// second address. Every record has it, minimal ones too.
		attrs = append(attrs, semconv.NetworkPeerAddress(c.peer))
	}
	opened := c.opened()
	attrs = append(attrs, attribute.Int64("hotline.timing.total_us", end.Sub(opened).Microseconds()))
	spans := c.spans(opened, end)
	if c.api != nil {
		attrs = append(attrs, c.full()...)
		attrs = append(attrs, traceSignature(c.trace, spans, c.api.SigningKey))
		attrs = append(attrs, signature(attrs, c.start, c.trace, c.api.SigningKey)...)
	}
	c.h.record(c.in, finished{
		sc: c.trace, at: c.start, attrs: attrs, spans: spans,
		overhead: c.vendorIn.Sub(c.start) + end.Sub(orTime(c.vendorOut, end)), reached: true,
	})
}

// refused records a request the proxy read but did not forward, with a
// minimal record and a trace of the call span alone. The vendor host is in
// it only when the request named one.
func (h *handler) refused(r *http.Request, start time.Time, ingress, id string, target *url.URL, status int) {
	attrs := []attribute.KeyValue{attribute.String("hotline.integration.id", orNotPassed(id))}
	if ingress != "" {
		attrs = append(attrs, attribute.String("hotline.ingress", ingress))
	}
	if target != nil && target.Hostname() != "" {
		attrs = append(attrs, semconv.ServerAddress(strings.ToLower(target.Hostname())))
	}
	end := h.clock.Now()
	attrs = append(attrs,
		semconv.HTTPRequestMethodKey.String(r.Method),
		semconv.HTTPResponseStatusCode(status),
		semconv.ErrorTypeKey.String("http_request_error"),
		attribute.Int64("hotline.timing.total_us", end.Sub(start).Microseconds()),
	)
	traced, parent, _ := h.traced(id, r.Header, start)
	spans := []export.Span{callSpan(traced, parent, start, end, true)}
	h.record(r, finished{sc: traced, at: start, attrs: attrs, spans: spans})
}

// finished is what a call leaves: its record, its trace and, once it got as
// far as the vendor, the proxy's own time on it.
type finished struct {
	sc       trace.SpanContext
	at       time.Time
	attrs    []attribute.KeyValue
	spans    []export.Span
	overhead time.Duration
	reached  bool
}

// record hands a record and its trace to the exporters, and measures the
// call for the proxy's own numbers.
func (h *handler) record(r *http.Request, f finished) {
	h.sink.Record(recordContext(f.sc), f.at, f.attrs)
	h.sink.Trace(f.sc, f.spans)
	h.own.ended(r, f.attrs, f.overhead, f.reached)
}

func orNotPassed(id string) string {
	if id == "" {
		return "not-passed"
	}
	return id
}

// full is the rest of the record, for calls with a known integration and api.
func (c *call) full() []attribute.KeyValue {
	origin := c.scheme + "://" + c.host
	if c.target.Port() != "" {
		origin += ":" + c.port
	}
	var replyHeaders map[string][]string
	if c.resp != nil {
		replyHeaders = c.resp.Header
	}
	redacted := c.api.Spec.Redact(spec.Call{
		Method:       c.in.Method,
		Origin:       origin,
		EscapedPath:  c.target.EscapedPath(),
		RawQuery:     c.target.RawQuery,
		Header:       c.in.Header,
		ReplyHeaders: replyHeaders,
	})
	port, _ := strconv.Atoi(c.port)
	sent, _ := c.reqBody.counts()
	attrs := []attribute.KeyValue{
		semconv.URLScheme(c.scheme),
		semconv.ServerPort(port),
		semconv.HTTPRequestBodySize(int(sent)),
		attribute.Bool("hotline.client.cancelled", c.app.gone),
		attribute.String("hotline.config.version", c.cfg.Label()),
	}
	if redacted.Template != "" {
		attrs = append(attrs, semconv.URLFull(redacted.URLFull), semconv.URLTemplate(redacted.Template))
	}
	if body := c.reqBody.signed(); body != "" {
		attrs = append(attrs, attribute.String("hotline.request.body.signature", body))
	}
	if c.resp != nil {
		if body := c.respBody.signed(); body != "" {
			attrs = append(attrs, attribute.String("hotline.response.body.signature", body))
		}
		got, _ := c.respBody.counts()
		attrs = append(attrs,
			semconv.NetworkProtocolVersion(strings.TrimSuffix(strings.TrimPrefix(c.resp.Proto, "HTTP/"), ".0")),
			semconv.HTTPResponseBodySize(int(got)))
	}
	if c.resp != nil && c.resp.TLS != nil {
		attrs = append(attrs,
			semconv.TLSProtocolVersion(strings.TrimPrefix(tls.VersionName(c.resp.TLS.Version), "TLS ")),
			semconv.TLSServerNotAfter(c.resp.TLS.PeerCertificates[0].NotAfter.UTC().Format(time.RFC3339)))
	}
	for name, values := range redacted.Header {
		attrs = append(attrs, attribute.StringSlice("http.request.header."+name, values))
	}
	for name, values := range redacted.ReplyHeaders {
		attrs = append(attrs, attribute.StringSlice("http.response.header."+name, values))
	}
	return append(attrs, c.checked()...)
}

// shownResults is how many results of one side a record holds at most.
const shownResults = 20

// checked checks the call against the spec and says what was found.
func (c *call) checked() []attribute.KeyValue {
	e := spec.Exchange{
		Method:      c.in.Method,
		EscapedPath: c.target.EscapedPath(),
		RawQuery:    c.target.RawQuery,
		Header:      c.in.Header,
		Body:        c.reqBody.copied(),
	}
	if c.resp != nil {
		e.Status, e.ReplyHeader, e.ReplyBody = c.resp.StatusCode, c.resp.Header, c.respBody.copied()
	}
	found := c.api.Spec.Check(e)
	var attrs []attribute.KeyValue
	for _, f := range found.Copied {
		switch v := f.Value.(type) {
		case string:
			attrs = append(attrs, attribute.String(f.Name, v))
		case bool:
			attrs = append(attrs, attribute.Bool(f.Name, v))
		case int64:
			attrs = append(attrs, attribute.Int64(f.Name, v))
		case float64:
			attrs = append(attrs, attribute.Float64(f.Name, v))
		}
	}
	if found.Skipped != "" {
		attrs = append(attrs, attribute.String("hotline.schema.skipped", found.Skipped))
	}
	for side, results := range map[string][]spec.Result{"request": found.Request, "response": found.Reply} {
		if len(results) == 0 {
			continue
		}
		key := "hotline.schema." + side + ".errors"
		if len(results) > shownResults {
			attrs = append(attrs, attribute.Int(key+"_total", len(results)))
			results = results[:shownResults]
		}
		values := make([]attribute.Value, len(results))
		for i, r := range results {
			entry := []attribute.KeyValue{attribute.String("part", r.Part), attribute.String("rule", r.Rule)}
			if r.Place != "" {
				entry = append(entry, attribute.String("place", r.Place))
			}
			if r.Expected != "" {
				entry = append(entry, attribute.String("expected", r.Expected))
			}
			values[i] = attribute.MapValue(entry...)
		}
		attrs = append(attrs, attribute.Slice(key, values...))
	}
	return attrs
}
