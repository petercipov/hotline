package proxy_test

import (
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/cucumber/godog"
)

func ingressSteps(sc *godog.ScenarioContext, w *world) {
	sc.Step(`^the proxy is running$`, func() error {
		w.appID = ""
		return w.start()
	})
	sc.Step(`^an app sends "([A-Z]+) ([^"]+)"$`, func(method, target string) error {
		return w.call(method, target)
	})
	sc.Step(`^an app sends "([A-Z]+) ([^"]+)" with the header "([^"]+)"$`, func(method, target, header string) error {
		return w.call(method, target, header)
	})
	sc.Step(`^the call uses the forward proxy ingress$`, func() error {
		record, err := w.record()
		if err != nil {
			return err
		}
		return has(record, "hotline.ingress", "forward_proxy")
	})
	sc.Step(`^the vendor is (\S+) on port (\d+)$`, func(host, port string) error {
		calls := w.vendor(host).received()
		if len(calls) != 1 || calls[0].port != port {
			return failf("%s got %d calls %+v, want one on port %s", host, len(calls), calls, port)
		}
		return nil
	})
	sc.Step(`^the vendor is (\S+)$`, func(host string) error {
		if calls := w.vendor(host).received(); len(calls) != 1 {
			return failf("%s got %d calls, want one", host, len(calls))
		}
		return nil
	})
	sc.Step(`^the proxy refuses the call$`, func() error {
		if w.resp == nil || w.resp.StatusCode != http.StatusBadRequest {
			return failf("the app got %v, want 400", w.resp)
		}
		return nil
	})
	sc.Step(`^nothing is sent to any vendor$`, w.nothingSent)
}

func (w *world) nothingSent() error {
	lookups, dials := w.network.counts()
	if lookups+dials > 0 {
		return failf("the proxy made %d lookups and %d connections, want none", lookups, dials)
	}
	return nil
}

func integrationSteps(sc *godog.ScenarioContext, w *world) {
	sc.Step(`^integration (\S+) with an api for (?:host )?(\S+)$`, func(id, host string) error {
		w.addIntegration(integration(id, w.apiFor(host, "")))
		return nil
	})
	sc.Step(`^an app calls https://(\S+) (with|without) (.+)$`, func(host, with, how string) error {
		target := "https://" + host + "/v2/customer_sessions/sess-1"
		w.appID = ""
		var headers []string
		switch {
		case with == "without":
		case strings.HasPrefix(how, `"Proxy-Authorization: `) && strings.Contains(how, `" for "`):
			scheme, credentials, _ := strings.Cut(strings.TrimPrefix(how, `"Proxy-Authorization: `), `" for "`)
			headers = append(headers, "Proxy-Authorization: "+scheme+" "+
				base64.StdEncoding.EncodeToString([]byte(strings.TrimSuffix(credentials, `"`))))
		case strings.HasPrefix(how, `the ID "`):
			w.appID = strings.Trim(strings.TrimPrefix(how, "the ID "), `"`)
		case strings.HasPrefix(how, `"`):
			headers = append(headers, strings.Trim(how, `"`))
		case how == "Basic credentials with no colon":
			headers = append(headers, basic("talon-shop-a"))
		case how == "Basic credentials with an empty user name":
			headers = append(headers, basic(":x"))
		case how == "two Proxy-Authorization headers":
			headers = append(headers, basic("talon-shop-a:x"), basic("talon-shop-a:x"))
		default:
			return godog.ErrUndefined
		}
		return w.call("PUT", target, headers...)
	})
	sc.Step(`^the integration ID is (\S+)$`, w.recordID)
	sc.Step(`^the record's ID is (\S+?)(?:, so typos stay visible)?$`, w.recordID)
	sc.Step(`^hotline.integration.id is (\S+)$`, w.recordID)
	sc.Step(`^the api for api.talon.one is used, with its spec, TLS origination setting and vendorTimeout$`, func() error {
		record, err := w.record()
		if err != nil {
			return err
		}
		return has(record, "url.template", "/v2/customer_sessions/{customerSessionId}")
	})
	sc.Step(`^the not-passed settings apply$`, func() error {
		record, err := w.record()
		if err != nil {
			return err
		}
		return hasNo(record, "url.full", "url.scheme", "hotline.config.version")
	})
	sc.Step(`^an app calls with a broken Proxy-Authorization header$`, func() error {
		w.appID = ""
		return w.call("PUT", w.target, "Proxy-Authorization: Basic s3cr3t-hdr!!")
	})
	sc.Step(`^its value appears in no log, error or record$`, func() error {
		return w.nowhere("s3cr3t-hdr")
	})
}

func (w *world) recordID(id string) error {
	record, err := w.record()
	if err != nil {
		return err
	}
	return has(record, "hotline.integration.id", id)
}

// nowhere checks that a value reached no record, log line, error or answer.
func (w *world) nowhere(values ...string) error {
	extra := string(w.respBody)
	if w.startErr != nil {
		extra += w.startErr.Error()
	}
	return w.notRecorded(values, extra)
}

// notRecorded checks that a value reached no record or log line, nor the
// extra text given.
func (w *world) notRecorded(values []string, extra ...string) error {
	var all strings.Builder
	for _, r := range w.records() {
		for k, v := range r.attrs {
			all.WriteString(k + "=" + strings.Join(strings.Fields(strings.Trim(sprint(v), "[]")), " ") + "\n")
		}
	}
	all.WriteString(w.logs.String())
	all.WriteString(strings.Join(extra, ""))
	for _, v := range values {
		if strings.Contains(all.String(), v) {
			return failf("%q appears in %s", v, all.String())
		}
	}
	return nil
}
