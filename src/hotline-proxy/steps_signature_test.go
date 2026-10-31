package proxy_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/cucumber/godog"
)

// signingKey is the integration's fingerprintKey in these scenarios.
const signingKey = "hotline-test-signing-key-1"

func signatureSteps(sc *godog.ScenarioContext, w *world) {
	var changed receivedRecord
	sc.Step(`^the integration's fingerprintKey is read from a file$`, func() error {
		file := w.writeFile("keys/talon-shop-a", signingKey+"\n")
		last := len(w.integrations) - 1
		w.integrations[last] += "    fingerprintKey: { file: " + file + " }\n"
		return nil
	})
	sc.Step(`^the shop spec, with no fingerprintKey$`, func() error {
		last := len(w.integrations) - 1
		before, _, found := strings.Cut(w.integrations[last], "    fingerprintKey:")
		if !found {
			return failf("the integration has no fingerprintKey: %s", w.integrations[last])
		}
		w.integrations[last] = before
		return nil
	})
	sc.Step(`^the record has hotline.signature and hotline.signature_key_id (\S+)$`, func(keyID string) error {
		if err := w.lastHasAll("hotline.signature"); err != nil {
			return err
		}
		return w.lastHas("hotline.signature_key_id", keyID)
	})
	sc.Step(`^the record has (hotline\.schema\.request\.errors)$`, func(name string) error {
		return w.lastHasAll(name)
	})
	sc.Step(`^hotline.request.body.signature is the HMAC of the body the vendor got, with the key$`, func() error {
		calls := w.vendor("api.shop.test").received()
		if len(calls) == 0 {
			return failf("the vendor got no call")
		}
		return w.lastHas("hotline.request.body.signature", hmacOf(calls[len(calls)-1].body))
	})
	sc.Step(`^hotline.response.body.signature is the HMAC of the body the app got, with the key$`, func() error {
		return w.lastHas("hotline.response.body.signature", hmacOf(w.respBody))
	})
	sc.Step(`^the record has no hotline.request.body.signature or hotline.response.body.signature$`, func() error {
		record, err := w.lastRecord()
		if err != nil {
			return err
		}
		return hasNo(record, "hotline.request.body.signature", "hotline.response.body.signature")
	})
	sc.Step(`^the record has no hotline.signature$`, func() error {
		record, err := w.lastRecord()
		if err != nil {
			return err
		}
		if _, signed := record["hotline.signature"]; signed {
			return failf("the minimal record is signed: %v", record)
		}
		return nil
	})
	sc.Step(`^the signature checks out when rebuilt from the record alone, with the key( talon-shop-a)?$`, func(byID string) error {
		key := signingKey
		if byID != "" {
			key = "talon-shop-a"
		}
		records := w.records()
		if len(records) == 0 {
			return failf("no record arrived")
		}
		return checkSignature(records[len(records)-1], key)
	})
	sc.Step(`^(.+) in the received record$`, func(change string) error {
		records := w.records()
		if len(records) == 0 {
			return failf("no record arrived")
		}
		last := records[len(records)-1]
		changed = last
		changed.attrs = map[string]any{}
		for k, v := range last.attrs {
			changed.attrs[k] = v
		}
		switch change {
		case "http.request.method is changed":
			changed.attrs["http.request.method"] = "PUT"
		case "url.full is changed":
			changed.attrs["url.full"] = "https://api.shop.test/orders?x=1"
		case "a kept header's value is changed":
			changed.attrs["http.request.header.content-type"] = []any{"text/plain"}
		case "the trace_id is changed":
			changed.traceID = strings.Repeat("ab", 16)
		case "the span_id is changed":
			changed.spanID = strings.Repeat("cd", 8)
		case "the flags are changed":
			changed.flags ^= 1
		case "http.response.status_code is changed":
			changed.attrs["http.response.status_code"] = int64(200)
		case "hotline.timing.total_us is changed":
			changed.attrs["hotline.timing.total_us"] = changed.attrs["hotline.timing.total_us"].(int64) + 1
		case "hotline.integration.id is changed":
			changed.attrs["hotline.integration.id"] = "talon-shop-b"
		case "shop.order.item is changed":
			changed.attrs["shop.order.item"] = "A2"
		case "hotline.trace.signature is changed":
			changed.attrs["hotline.trace.signature"] = hmacOf([]byte("{}"))
		case "hotline.request.body.signature is changed":
			changed.attrs["hotline.request.body.signature"] = hmacOf([]byte("{}"))
		case "an attribute is added":
			changed.attrs["shop.added"] = "x"
		case "an attribute is removed":
			delete(changed.attrs, "hotline.ingress")
		case "the timestamp is changed":
			changed.timestamp++
		default:
			return godog.ErrPending
		}
		return nil
	})
	sc.Step(`^the signature no longer checks out$`, func() error {
		if checkSignature(changed, signingKey) == nil {
			return failf("the changed record still checks out")
		}
		return nil
	})
	var changedTrace callTrace
	sc.Step(`^the record has (no )?hotline.trace.signature$`, func(no string) error {
		record, err := w.lastRecord()
		if err != nil {
			return err
		}
		if no != "" {
			return hasNo(record, "hotline.trace.signature")
		}
		return hasAll(record, "hotline.trace.signature")
	})
	sc.Step(`^the trace signature checks out when rebuilt from the spans alone, with the key$`, func() error {
		t, err := w.lastTrace()
		if err != nil {
			return err
		}
		return checkTraceSignature(t, signingKey)
	})
	sc.Step(`^(.+) in the received trace$`, func(change string) error {
		t, err := w.lastTrace()
		if err != nil {
			return err
		}
		changedTrace = callTrace{record: t.record}
		for _, s := range t.spans {
			s.attrs = maps.Clone(s.attrs)
			changedTrace.spans = append(changedTrace.spans, s)
		}
		last := len(changedTrace.spans) - 1
		victim := &changedTrace.spans[last]
		switch change {
		case "a span's end is changed":
			victim.end++
		case "a span's name is changed":
			victim.name += " too"
		case "a span's parent is changed":
			victim.parent = strings.Repeat("ab", 8)
		case "a span's status is changed":
			victim.status = map[string]string{"error": "unset", "unset": "error"}[victim.status]
		case "an attribute is changed":
			for i, s := range changedTrace.spans {
				if s.name == "vendor call" {
					changedTrace.spans[i].attrs["hotline.connection.reused"] = s.attrs["hotline.connection.reused"] != true
				}
			}
		case "a span is removed":
			changedTrace.spans = changedTrace.spans[:last]
		default:
			return godog.ErrPending
		}
		return nil
	})
	sc.Step(`^the trace signature no longer checks out$`, func() error {
		if checkTraceSignature(changedTrace, signingKey) == nil {
			return failf("the changed trace still checks out")
		}
		return nil
	})
	sc.Step(`^the record holds no (\S+)$`, func(secret string) error {
		record, err := w.lastRecord()
		if err != nil {
			return err
		}
		if strings.Contains(fmt.Sprint(record), secret) {
			return failf("the record holds %s: %v", secret, record)
		}
		return nil
	})
	sc.Step(`^the vendor got no Signature or Signature-Input header$`, func() error {
		for _, c := range w.vendor("api.shop.test").received() {
			if c.header.Get("Signature") != "" || c.header.Get("Signature-Input") != "" {
				return failf("the vendor got %v", c.header)
			}
		}
		return nil
	})
	sc.Step(`^the app got no Signature or Signature-Input header$`, func() error {
		if w.resp == nil {
			return failf("the app got no reply")
		}
		if w.resp.Header.Get("Signature") != "" || w.resp.Header.Get("Signature-Input") != "" {
			return failf("the app got %v", w.resp.Header)
		}
		return nil
	})
	sc.Step(`^the app sends GET /orders with no integration ID$`, func() error {
		w.appID = ""
		return w.shopCall("GET", "/orders", true, "", "", "")
	})
}

// hmacOf is a body's signature, as the proxy makes it with the key.
func hmacOf(body []byte) string {
	mac := hmac.New(sha256.New, []byte(signingKey))
	mac.Write(body)
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// checkSignature rebuilds the signed text from a record as
// docs/proxy-signatures.md describes it, and checks the signature with the
// key, the way any checker that has only the record would.
func checkSignature(r receivedRecord, key string) error {
	signature, _ := r.attrs["hotline.signature"].(string)
	lines := make([]string, 0, len(r.attrs))
	for name, v := range r.attrs {
		if name == "hotline.signature" {
			continue
		}
		value, err := json.Marshal(v)
		if err != nil {
			return err
		}
		lines = append(lines, strconv.Quote(name)+": "+string(value))
	}
	sort.Strings(lines)
	lines = append(lines,
		`"log.record.timestamp": `+strconv.FormatUint(r.timestamp, 10),
		`"log.record.trace_id": "`+r.traceID+`"`,
		`"log.record.span_id": "`+r.spanID+`"`,
		`"log.record.trace_flags": "`+fmt.Sprintf("%02x", r.flags)+`"`)
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(strings.Join(lines, "\n")))
	if want := base64.StdEncoding.EncodeToString(mac.Sum(nil)); signature != want {
		return failf("the signature %q does not check out", signature)
	}
	return nil
}

// checkTraceSignature rebuilds a trace's signed text from its spans as
// docs/proxy-traces.md describes it, and checks it against the record's
// hotline.trace.signature with the key.
func checkTraceSignature(t callTrace, key string) error {
	signature, _ := t.record.attrs["hotline.trace.signature"].(string)
	spans := slices.Clone(t.spans)
	sort.Slice(spans, func(i, j int) bool {
		if spans[i].start != spans[j].start {
			return spans[i].start < spans[j].start
		}
		return spans[i].spanID < spans[j].spanID
	})
	line := func(name string, v any) string {
		// The receiver's values are plain types and finite numbers.
		value, _ := json.Marshal(v) //nolint:errchkjson // see above
		return strconv.Quote(name) + ": " + string(value)
	}
	lines := []string{line("trace.trace_id", t.record.traceID)}
	for _, s := range spans {
		lines = append(lines,
			line("span.span_id", s.spanID),
			line("span.parent_span_id", s.parent),
			line("span.name", s.name),
			line("span.kind", s.kind),
			line("span.start_time", s.start),
			line("span.end_time", s.end),
			line("span.status", s.status))
		var attrs []string
		for name, v := range s.attrs {
			attrs = append(attrs, line(name, v))
		}
		sort.Strings(attrs)
		lines = append(lines, attrs...)
	}
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(strings.Join(lines, "\n")))
	if want := base64.StdEncoding.EncodeToString(mac.Sum(nil)); signature != want {
		return failf("the trace signature %q does not check out", signature)
	}
	return nil
}
