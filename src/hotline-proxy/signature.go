package proxy

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// signature signs everything a full record holds, so anyone with the record
// and the key can check that nobody changed it. The signed text has one
// line per attribute, sorted by name, then one for the record's timestamp:
// the name and the value, both as JSON, and lines for its trace id, span id
// and flags, in hex. The whole record is signed, so no list of names is
// needed (Peter, 2026-10-31).
func signature(attrs []attribute.KeyValue, at time.Time, sc trace.SpanContext, key []byte) []attribute.KeyValue {
	sum := sha256.Sum256(key)
	attrs = append(slices.Clone(attrs), attribute.String("hotline.signature_key_id", hex.EncodeToString(sum[:8])))
	slices.SortFunc(attrs, func(a, b attribute.KeyValue) int { return strings.Compare(string(a.Key), string(b.Key)) })
	lines := make([]string, 0, len(attrs)+1)
	for _, kv := range attrs {
		// The values are plain types and finite numbers, so they always
		// turn into JSON.
		value, _ := json.Marshal(plain(kv.Value)) //nolint:errchkjson // see above
		lines = append(lines, strconv.Quote(string(kv.Key))+": "+string(value))
	}
	lines = append(lines,
		`"log.record.timestamp": `+strconv.FormatInt(at.UnixNano(), 10),
		`"log.record.trace_id": "`+sc.TraceID().String()+`"`,
		`"log.record.span_id": "`+sc.SpanID().String()+`"`,
		`"log.record.trace_flags": "`+sc.TraceFlags().String()+`"`)
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(strings.Join(lines, "\n")))
	return []attribute.KeyValue{
		attribute.String("hotline.signature_key_id", hex.EncodeToString(sum[:8])),
		attribute.String("hotline.signature", base64.StdEncoding.EncodeToString(mac.Sum(nil))),
	}
}

// plain turns an attribute value into the Go value JSON writes the same way
// a record's reader sees it: maps by key, lists in order.
func plain(v attribute.Value) any {
	switch v.Type() {
	case attribute.SLICE:
		items := v.AsSlice()
		out := make([]any, len(items))
		for i, item := range items {
			out[i] = plain(item)
		}
		return out
	case attribute.MAP:
		out := map[string]any{}
		for _, kv := range v.AsMap() {
			out[string(kv.Key)] = plain(kv.Value)
		}
		return out
	}
	return v.AsInterface()
}
