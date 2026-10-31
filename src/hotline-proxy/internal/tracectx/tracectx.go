// Package tracectx reads the trace context an app sends with its call, in
// one of four formats, and writes the proxy's own in the same format. A
// trace is all the steps of one request, across services, tied together by
// one trace id; a span is one of those steps.
//
// The app's span id is read too: the proxy's own spans sit under it. It is
// not on the record (Peter, 2026-10-31).
package tracectx

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel/trace"
)

// Format is how an app sends its trace context.
type Format string

// The formats an integration may pick with traceFormat.
const (
	// W3C is the traceparent header of W3C Trace Context, the default.
	W3C Format = "w3c"
	// B3 is Zipkin's b3 header, or its X-B3-* headers.
	B3 Format = "b3"
	// XRay is the X-Amzn-Trace-Id header of AWS X-Ray.
	XRay Format = "xray"
	// Jaeger is Jaeger's uber-trace-id header.
	Jaeger Format = "jaeger"
)

// Parse finds the format a config names.
func Parse(name string) (Format, bool) {
	switch f := Format(name); f {
	case W3C, B3, XRay, Jaeger:
		return f, true
	}
	return "", false
}

// Read finds the app's trace id, span id and flags. It says false when the
// headers are missing or broken in any way, so the proxy makes a new trace
// id. The span id is not valid when the app sent none: X-Ray may leave it
// out.
func (f Format) Read(h http.Header) (trace.SpanContext, bool) {
	switch f {
	case B3:
		return readB3(h)
	case XRay:
		return readXRay(h)
	case Jaeger:
		return readJaeger(h)
	}
	return readW3C(h)
}

// Write puts the proxy's trace context on a call to the vendor, in place of
// the app's.
func (f Format) Write(h http.Header, sc trace.SpanContext) {
	traceID, spanID := sc.TraceID().String(), sc.SpanID().String()
	sampled := "0"
	if sc.IsSampled() {
		sampled = "1"
	}
	switch f {
	case B3:
		for _, name := range []string{"b3", "X-B3-ParentSpanId", "X-B3-Flags"} {
			h.Del(name)
		}
		h.Set("X-B3-TraceId", traceID)
		h.Set("X-B3-SpanId", spanID)
		h.Set("X-B3-Sampled", sampled)
	case XRay:
		h.Set("X-Amzn-Trace-Id", "Root=1-"+traceID[:8]+"-"+traceID[8:]+";Parent="+spanID+";Sampled="+sampled)
	case Jaeger:
		h.Set("uber-trace-id", traceID+":"+spanID+":0:"+sampled)
	default:
		h.Set("traceparent", "00-"+traceID+"-"+spanID+"-"+sc.TraceFlags().String())
	}
}

// NewTraceID makes a random trace id. For X-Ray its first four bytes are
// the time in seconds, as X-Ray needs.
func (f Format) NewTraceID(now time.Time) trace.TraceID {
	var id trace.TraceID
	_, _ = rand.Read(id[:]) // never fails
	if f == XRay {
		binary.BigEndian.PutUint32(id[:4], uint32(now.Unix())) //nolint:gosec // X-Ray holds the seconds in 32 bits
	}
	return id
}

// NewSpanID makes a random span id, for the proxy's own step.
func NewSpanID() trace.SpanID {
	var id trace.SpanID
	_, _ = rand.Read(id[:]) // never fails
	return id
}

// readW3C reads "traceparent: 00-<trace id>-<span id>-<flags>". A later
// version may add more parts at the end.
func readW3C(h http.Header) (trace.SpanContext, bool) {
	value, ok := one(h, "traceparent")
	parts := strings.Split(value, "-")
	if !ok || len(parts) < 4 || !isHex(parts[0], 2) || parts[0] == "ff" || (parts[0] == "00" && len(parts) != 4) ||
		!isID(parts[2], 16) || !isHex(parts[3], 2) {
		return trace.SpanContext{}, false
	}
	return spanContext(parts[1], parts[2], parts[3])
}

// readB3 reads "b3: <trace id>-<span id>[-<sampled>[-<parent span id>]]",
// or, without it, the X-B3-* headers. A trace id may be 16 hex digits long.
func readB3(h http.Header) (trace.SpanContext, bool) {
	var id, span, sampled, debug string
	if value, ok := one(h, "b3"); ok {
		parts := strings.Split(value, "-")
		if len(parts) < 2 || len(parts) > 4 || (len(parts) == 4 && !isID(parts[3], 16)) {
			return trace.SpanContext{}, false
		}
		parts = append(parts, "")
		id, span, sampled = parts[0], parts[1], parts[2]
	} else {
		id, _ = one(h, "X-B3-TraceId")
		span, _ = one(h, "X-B3-SpanId")
		sampled, _ = one(h, "X-B3-Sampled")
		debug, _ = one(h, "X-B3-Flags")
	}
	flags, known := map[string]string{"": "00", "0": "00", "false": "00", "1": "01", "true": "01", "d": "01"}[sampled]
	if debug == "1" {
		flags = "01"
	}
	if len(id) == 16 {
		id = strings.Repeat("0", 16) + id
	}
	if !known || !isID(span, 16) {
		return trace.SpanContext{}, false
	}
	return spanContext(id, span, flags)
}

// readXRay reads "X-Amzn-Trace-Id: Root=1-<time>-<id>;Parent=<span id>;Sampled=1",
// its parts in any order, and others among them.
func readXRay(h http.Header) (trace.SpanContext, bool) {
	value, _ := one(h, "X-Amzn-Trace-Id")
	parts := map[string]string{}
	for _, part := range strings.Split(value, ";") {
		key, v, _ := strings.Cut(strings.TrimSpace(part), "=")
		parts[key] = v
	}
	root := strings.Split(parts["Root"], "-")
	parent, hasParent := parts["Parent"]
	flags, known := map[string]string{"": "00", "0": "00", "?": "00", "1": "01"}[parts["Sampled"]]
	if len(root) != 3 || !known || root[0] != "1" || !isHex(root[1], 8) || (hasParent && !isID(parent, 16)) {
		return trace.SpanContext{}, false
	}
	return spanContext(root[1]+root[2], parent, flags)
}

// readJaeger reads "uber-trace-id: <trace id>:<span id>:<parent>:<flags>",
// which may be URL-encoded. The ids may leave out leading zeros. The flags
// are sampled when either the sampled or the debug bit is set.
func readJaeger(h http.Header) (trace.SpanContext, bool) {
	value, _ := one(h, "uber-trace-id")
	value, err := url.PathUnescape(value)
	parts := strings.Split(value, ":")
	if err != nil || len(parts) != 4 || len(parts[0]) > 32 || len(parts[1]) > 16 ||
		!isID(strings.Repeat("0", 16-len(parts[1]))+parts[1], 16) {
		return trace.SpanContext{}, false
	}
	bits, err := strconv.ParseUint(parts[3], 16, 8)
	if err != nil {
		return trace.SpanContext{}, false
	}
	flags := "00"
	if bits&3 != 0 {
		flags = "01"
	}
	return spanContext(strings.Repeat("0", 32-len(parts[0]))+parts[0], strings.Repeat("0", 16-len(parts[1]))+parts[1], flags)
}

// spanContext turns a trace id, a span id and flags, all in hex, into their
// values. An empty span id stays not valid.
func spanContext(id, span, flags string) (trace.SpanContext, bool) {
	var t trace.TraceID
	if !isID(id, 32) {
		return trace.SpanContext{}, false
	}
	var s trace.SpanID
	var f [1]byte
	_, _ = hex.Decode(t[:], []byte(id))
	_, _ = hex.Decode(s[:], []byte(span))
	_, _ = hex.Decode(f[:], []byte(flags))
	return trace.NewSpanContext(trace.SpanContextConfig{TraceID: t, SpanID: s, TraceFlags: trace.TraceFlags(f[0]), Remote: true}), true
}

// one is a header's value when the call holds it exactly once.
func one(h http.Header, name string) (string, bool) {
	values := h.Values(name)
	if len(values) != 1 {
		return "", false
	}
	return values[0], true
}

// isID says whether s is n lower-case hex digits, not all zero.
func isID(s string, n int) bool {
	return isHex(s, n) && strings.Trim(s, "0") != ""
}

// isHex says whether s is n lower-case hex digits.
func isHex(s string, n int) bool {
	if len(s) != n {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
