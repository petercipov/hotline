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
		case "shop.order.item is changed":
			changed.attrs["shop.order.item"] = "A2"
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
	lines = append(lines, `"log.record.timestamp": `+strconv.FormatUint(r.timestamp, 10))
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(strings.Join(lines, "\n")))
	if want := base64.StdEncoding.EncodeToString(mac.Sum(nil)); signature != want {
		return failf("the signature %q does not check out", signature)
	}
	return nil
}
