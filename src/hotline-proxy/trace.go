package proxy

import (
	"cmp"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"

	"hotline-proxy/internal/export"
	"hotline-proxy/internal/tracectx"
)

// opened is when the call's time starts: when it reached the proxy or, on
// the first call of an app's TLS connection, when TLS with the app started
// to be set up (Peter, 2026-10-31).
func (c *call) opened() time.Time {
	if c.appTimed {
		return c.appStart
	}
	return c.start
}

// spans are the steps of the call, as its trace (Peter, 2026-10-31). A step
// that did not happen has no span. Spans hold nothing the record holds, so
// only two of them have an attribute.
func (c *call) spans(opened, end time.Time) []export.Span {
	call := callSpan(c.trace, c.parent, opened, end, c.errorType != "" || c.app.failed())
	spans := []export.Span{call}
	step := func(parent trace.SpanID, name string, from, to time.Time, attrs ...attribute.KeyValue) {
		spans = append(spans, export.Span{
			ID: tracectx.NewSpanID(), Parent: parent, Name: name, Kind: trace.SpanKindInternal,
			Start: from, End: to, Failed: name == c.failedStep, Attrs: attrs,
		})
	}
	if c.appTimed {
		step(call.ID, "app tls handshake", c.appStart, c.appEnd)
	}
	step(call.ID, "routing", c.start, c.vendorIn)
	// The reply ends when the vendor's reply ended, or when the call
	// failed.
	out := orTime(c.vendorOut, end)
	vendor := export.Span{
		ID: c.vendorSpan, Parent: call.ID, Name: "vendor call", Kind: trace.SpanKindClient,
		Start: c.vendorIn, End: out, Failed: c.errorType != "",
	}
	if c.connected {
		vendor.Attrs = []attribute.KeyValue{attribute.Bool("hotline.connection.reused", c.reused)}
	}
	spans = append(spans, vendor)
	for _, p := range []struct {
		phase phase
		name  string
	}{{phaseDNS, "dns lookup"}, {phaseConnect, "tcp connect"}, {phaseTLS, "vendor tls handshake"}} {
		t := c.times[p.phase]
		if t[0].IsZero() {
			continue
		}
		if t[1].IsZero() {
			t[1] = out
		}
		var attrs []attribute.KeyValue
		if p.phase == phaseDNS && c.looked != "" {
			attrs = append(attrs, semconv.DNSQuestionName(c.looked))
		}
		step(vendor.ID, p.name, t[0], t[1], attrs...)
	}
	if !c.gotAt.IsZero() {
		step(vendor.ID, "send call", c.gotAt, orTime(c.wrote, out))
	}
	if !c.wrote.IsZero() && (c.firstByte.IsZero() || !c.firstByte.Before(c.wrote)) {
		step(vendor.ID, "wait for vendor", c.wrote, orTime(c.firstByte, out))
	}
	if !c.firstByte.IsZero() {
		step(vendor.ID, "receive reply", c.firstByte, out)
	}
	step(call.ID, "finish reply", out, end)
	// The app hung up: the vendor did nothing wrong, so only the last step
	// fails (Peter, 2026-10-31).
	spans[len(spans)-1].Failed = c.app.failed()
	return spans
}

// orTime is t, or instead when t is not set.
func orTime(t, instead time.Time) time.Time {
	if t.IsZero() {
		return instead
	}
	return t
}

// callSpan is the span of the whole call, under the app's span when the app
// sent one. Its id is the record's span id, which links the two.
func callSpan(sc trace.SpanContext, parent trace.SpanID, from, to time.Time, failed bool) export.Span {
	return export.Span{
		ID: sc.SpanID(), Parent: parent, Name: "hotline call", Kind: trace.SpanKindServer,
		Start: from, End: to, Failed: failed,
	}
}

// traceSignature signs a call's trace with the integration's key, so the
// record, which carries it, covers the trace too (Peter, 2026-10-31). The
// signed text is a line with the trace id, then one block per span in start
// order, a tie going by span id: its span id, parent span id, name, kind,
// start, end and status, then its attributes sorted by name. Each line is a
// name and a value, both as JSON.
func traceSignature(sc trace.SpanContext, spans []export.Span, key []byte) attribute.KeyValue {
	spans = slices.Clone(spans)
	slices.SortFunc(spans, func(a, b export.Span) int {
		if c := cmp.Compare(a.Start.UnixNano(), b.Start.UnixNano()); c != 0 {
			return c
		}
		return strings.Compare(a.ID.String(), b.ID.String())
	})
	line := func(name string, v any) string {
		// The values are plain types and finite numbers, so they always
		// turn into JSON.
		value, _ := json.Marshal(v) //nolint:errchkjson // see above
		return strconv.Quote(name) + ": " + string(value)
	}
	lines := []string{line("trace.trace_id", sc.TraceID().String())}
	for _, s := range spans {
		parent, status := "", "unset"
		if s.Parent.IsValid() {
			parent = s.Parent.String()
		}
		if s.Failed {
			status = "error"
		}
		if s.OK {
			status = "ok"
		}
		kind := map[trace.SpanKind]string{trace.SpanKindServer: "server", trace.SpanKindClient: "client"}[s.Kind]
		if kind == "" {
			kind = "internal"
		}
		lines = append(lines,
			line("span.span_id", s.ID.String()),
			line("span.parent_span_id", parent),
			line("span.name", s.Name),
			line("span.kind", kind),
			line("span.start_time", s.Start.UnixNano()),
			line("span.end_time", s.End.UnixNano()),
			line("span.status", status))
		attrs := make([]string, 0, len(s.Attrs))
		for _, kv := range s.Attrs {
			attrs = append(attrs, line(string(kv.Key), plain(kv.Value)))
		}
		slices.Sort(attrs)
		lines = append(lines, attrs...)
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(strings.Join(lines, "\n")))
	return attribute.String("hotline.trace.signature", base64.StdEncoding.EncodeToString(mac.Sum(nil)))
}
