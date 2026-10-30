package proxy

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"

	"hotline/proxy/internal/spec"
)

// finish builds the call's record once the reply has gone back to the app,
// and hands it to the exporters without waiting for them.
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
			c.errorType = "http_response_incomplete"
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
	attrs = append(attrs, c.timings(end)...)
	if c.api != nil {
		attrs = append(attrs, c.full()...)
	}
	c.h.sink.Record(context.Background(), c.start, attrs)
}

// refused records a request the proxy read but did not forward, with a
// minimal record. The vendor host is in it only when the request named one.
func (h *handler) refused(r *http.Request, start time.Time, ingress, id string, target *url.URL, status int) {
	attrs := []attribute.KeyValue{attribute.String("hotline.integration.id", orNotPassed(id))}
	if ingress != "" {
		attrs = append(attrs, attribute.String("hotline.ingress", ingress))
	}
	if target != nil && target.Hostname() != "" {
		attrs = append(attrs, semconv.ServerAddress(strings.ToLower(target.Hostname())))
	}
	attrs = append(attrs,
		semconv.HTTPRequestMethodKey.String(r.Method),
		semconv.HTTPResponseStatusCode(status),
		semconv.ErrorTypeKey.String("http_request_error"),
		attribute.Float64("hotline.timing.total", h.clock.Now().Sub(start).Seconds()),
	)
	h.sink.Record(context.Background(), start, attrs)
}

func orNotPassed(id string) string {
	if id == "" {
		return "not-passed"
	}
	return id
}

// timings are the phase lengths in seconds. A phase that did not happen,
// such as DNS for a reused connection, has no timing.
func (c *call) timings(end time.Time) []attribute.KeyValue {
	seconds := func(from, to time.Time) float64 { return to.Sub(from).Seconds() }
	var attrs []attribute.KeyValue
	for _, p := range []phase{phaseDNS, phaseConnect, phaseTLS} {
		if t := c.times[p]; !t[1].IsZero() {
			attrs = append(attrs, attribute.Float64("hotline.timing."+string(p), seconds(t[0], t[1])))
		}
	}
	if c.appTimed {
		attrs = append(attrs, attribute.Float64("hotline.timing.app_tls", c.appTLS.Seconds()))
	}
	if !c.firstByte.IsZero() {
		attrs = append(attrs, attribute.Float64("hotline.timing.first_byte", seconds(c.start, c.firstByte)))
	}
	return append(attrs,
		attribute.Float64("hotline.timing.total", seconds(c.start, end)),
		attribute.Float64("hotline.timing.proxy_request", seconds(c.start, c.vendorIn)),
		attribute.Float64("hotline.timing.proxy_response", seconds(c.vendorOut, end)),
	)
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
		attribute.Bool("hotline.connection.reused", c.reused),
		attribute.String("hotline.config.version", c.h.cfg.Version),
	}
	if redacted.Template != "" {
		attrs = append(attrs, semconv.URLFull(redacted.URLFull), semconv.URLTemplate(redacted.Template))
	}
	if c.looked != "" {
		attrs = append(attrs, semconv.DNSQuestionName(c.looked))
	}
	if c.resp != nil {
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
