package proxy

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"

	"hotline-proxy/internal/httpsig"
)

// signature signs everything a full record holds, so anyone with the record
// and the key can check that nobody changed it. The RFC 9421 lines for the
// call come first: the method, the address, the kept request headers, the
// status and the kept reply headers, marked ;resp. Then one line for each
// other attribute and for the record's timestamp, sorted by name, with its
// value as JSON.
func signature(attrs []attribute.KeyValue, at, now time.Time, key []byte) []attribute.KeyValue {
	var method, target, status []httpsig.Component
	var request, reply, rest []httpsig.Component
	for _, kv := range attrs {
		name := string(kv.Key)
		switch {
		case kv.Key == semconv.HTTPRequestMethodKey:
			method = []httpsig.Component{{Name: "@method", Value: kv.Value.AsString()}}
		case kv.Key == semconv.URLFullKey:
			target = []httpsig.Component{{Name: "@target-uri", Value: kv.Value.AsString()}}
		case kv.Key == semconv.HTTPResponseStatusCodeKey:
			status = []httpsig.Component{{Name: "@status", Value: strconv.FormatInt(kv.Value.AsInt64(), 10)}}
		case strings.HasPrefix(name, "http.request.header."):
			request = append(request, httpsig.Component{
				Name: strings.TrimPrefix(name, "http.request.header."), Value: strings.Join(kv.Value.AsStringSlice(), ", "),
			})
		case strings.HasPrefix(name, "http.response.header."):
			reply = append(reply, httpsig.Component{
				Name: strings.TrimPrefix(name, "http.response.header."), Param: "resp", Value: strings.Join(kv.Value.AsStringSlice(), ", "),
			})
		default:
			// The values are plain types and finite numbers, so they always
			// turn into JSON.
			value, _ := json.Marshal(plain(kv.Value)) //nolint:errchkjson // see above
			rest = append(rest, httpsig.Component{Name: name, Value: string(value)})
		}
	}
	rest = append(rest, httpsig.Component{Name: "log.record.timestamp", Value: strconv.FormatInt(at.UnixNano(), 10)})
	for _, part := range [][]httpsig.Component{request, reply, rest} {
		slices.SortFunc(part, func(a, b httpsig.Component) int { return strings.Compare(a.Name, b.Name) })
	}
	components := slices.Concat(method, target, request, status, reply, rest)
	sum := sha256.Sum256(key)
	value, input := httpsig.HMACSHA256(key, components, httpsig.Params{
		Created: now.Unix(), KeyID: hex.EncodeToString(sum[:8]), Alg: "hmac-sha256",
	})
	return []attribute.KeyValue{
		attribute.String("hotline.signature", value),
		attribute.String("hotline.signature_input", input),
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
