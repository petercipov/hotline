package proxy_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
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
	sc.Step(`^the record has hotline.signature and hotline.signature_input$`, func() error {
		return w.lastHasAll("hotline.signature", "hotline.signature_input")
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
	sc.Step(`^the signature input starts with (.+)$`, func(want string) error {
		covered, _, err := w.signatureInput()
		if err != nil {
			return err
		}
		if !strings.HasPrefix(covered, want+" ") {
			return failf("the signature input covers %s", covered)
		}
		return nil
	})
	sc.Step(`^the signature input names ("[^"]+";resp)$`, func(want string) error {
		covered, _, err := w.signatureInput()
		if err != nil {
			return err
		}
		if !strings.Contains(" "+covered+" ", " "+want+" ") {
			return failf("the signature input covers %s", covered)
		}
		return nil
	})
	sc.Step(`^the signature input names (hotline\.\S+)$`, func(name string) error {
		covered, _, err := w.signatureInput()
		if err != nil {
			return err
		}
		if !strings.Contains(covered, strconv.Quote(name)) {
			return failf("the signature input covers %s", covered)
		}
		return nil
	})
	sc.Step(`^the signature input names every other attribute and log.record.timestamp$`, func() error {
		record, err := w.lastRecord()
		if err != nil {
			return err
		}
		covered, _, err := w.signatureInput()
		if err != nil {
			return err
		}
		for name := range record {
			if name == "hotline.signature" || name == "hotline.signature_input" || isCallPart(name) {
				continue
			}
			if !strings.Contains(covered, strconv.Quote(name)) {
				return failf("the signature input leaves out %s: %s", name, covered)
			}
		}
		if !strings.Contains(covered, `"log.record.timestamp"`) {
			return failf("the signature input leaves out the timestamp: %s", covered)
		}
		return nil
	})
	sc.Step(`^the signature input says keyid (\S+) and alg (\S+)$`, func(keyID, alg string) error {
		_, params, err := w.signatureInput()
		if err != nil {
			return err
		}
		if !strings.Contains(params, `;keyid="`+keyID+`"`) || !strings.Contains(params, `;alg="`+alg+`"`) ||
			!strings.Contains(params, ";created=") {
			return failf("the signature parameters are %s", params)
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
		changed = receivedRecord{timestamp: last.timestamp, attrs: map[string]any{}}
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
		case "http.response.status_code is changed":
			changed.attrs["http.response.status_code"] = int64(200)
		case "hotline.timing.total is changed":
			changed.attrs["hotline.timing.total"] = changed.attrs["hotline.timing.total"].(float64) + 0.001
		case "hotline.integration.id is changed":
			changed.attrs["hotline.integration.id"] = "talon-shop-b"
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
	sc.Step(`^the signature input names no authorization, proxy-authorization, cookie or body$`, func() error {
		covered, _, err := w.signatureInput()
		if err != nil {
			return err
		}
		for _, name := range []string{"authorization", "proxy-authorization", "cookie", "content-digest", "body"} {
			if strings.Contains(covered, strconv.Quote(name)) {
				return failf("the signature input covers %s: %s", name, covered)
			}
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

// signatureInput splits the record's signature input into the covered
// list, without its brackets, and the parameters after it.
func (w *world) signatureInput() (covered, params string, err error) {
	record, err := w.lastRecord()
	if err != nil {
		return "", "", err
	}
	input, _ := record["hotline.signature_input"].(string)
	end := strings.Index(input, ")")
	if !strings.HasPrefix(input, "(") || end < 0 {
		return "", "", failf("the signature input is %q", input)
	}
	return input[1:end], input[end+1:], nil
}

func isCallPart(name string) bool {
	return name == "http.request.method" || name == "url.full" || name == "http.response.status_code" ||
		strings.HasPrefix(name, "http.request.header.") || strings.HasPrefix(name, "http.response.header.")
}

// checkSignature rebuilds the signature base from a record as
// docs/proxy-signatures.md describes it, and checks the signature with the
// key, the way any checker that has only the record would.
func checkSignature(r receivedRecord, key string) error {
	input, _ := r.attrs["hotline.signature_input"].(string)
	signature, _ := r.attrs["hotline.signature"].(string)
	end := strings.Index(input, ")")
	if !strings.HasPrefix(input, "(") || end < 0 {
		return failf("the signature input is %q", input)
	}
	type line struct{ name, value string }
	var call, reqHeaders, respHeaders, rest []line
	headerValue := func(v any) string {
		var parts []string
		for _, item := range v.([]any) {
			parts = append(parts, item.(string))
		}
		return strings.Join(parts, ", ")
	}
	for name, v := range r.attrs {
		switch {
		case name == "hotline.signature" || name == "hotline.signature_input":
		case strings.HasPrefix(name, "http.request.header."):
			reqHeaders = append(reqHeaders, line{strconv.Quote(strings.TrimPrefix(name, "http.request.header.")), headerValue(v)})
		case strings.HasPrefix(name, "http.response.header."):
			respHeaders = append(respHeaders, line{strconv.Quote(strings.TrimPrefix(name, "http.response.header.")) + ";resp", headerValue(v)})
		case isCallPart(name):
		default:
			value, err := json.Marshal(v)
			if err != nil {
				return err
			}
			rest = append(rest, line{strconv.Quote(name), string(value)})
		}
	}
	rest = append(rest, line{`"log.record.timestamp"`, strconv.FormatUint(r.timestamp, 10)})
	byName := func(lines []line) { sort.Slice(lines, func(i, j int) bool { return lines[i].name < lines[j].name }) }
	byName(reqHeaders)
	byName(respHeaders)
	byName(rest)
	call = append(call, line{`"@method"`, fmt.Sprint(r.attrs["http.request.method"])})
	if u, ok := r.attrs["url.full"]; ok {
		call = append(call, line{`"@target-uri"`, fmt.Sprint(u)})
	}
	call = append(call, reqHeaders...)
	if status, ok := r.attrs["http.response.status_code"]; ok {
		call = append(call, line{`"@status"`, fmt.Sprint(status)})
	}
	call = append(call, respHeaders...)
	call = append(call, rest...)
	var base strings.Builder
	names := make([]string, len(call))
	for i, l := range call {
		names[i] = l.name
		base.WriteString(l.name + ": " + l.value + "\n")
	}
	if got := strings.Join(names, " "); got != input[1:end] {
		return failf("the record holds %s, the signature covers %s", got, input[1:end])
	}
	base.WriteString(`"@signature-params": ` + input)
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(base.String()))
	if want := base64.StdEncoding.EncodeToString(mac.Sum(nil)); signature != want {
		return failf("the signature %q does not check out", signature)
	}
	return nil
}
