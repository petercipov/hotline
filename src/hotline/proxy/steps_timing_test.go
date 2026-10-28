package proxy_test

import (
	"io"
	"net/http"
	"net/netip"
	"time"

	"github.com/cucumber/godog"
)

func timingSteps(sc *godog.ScenarioContext, w *world) {
	sc.Step(`^a vendor reached over a new https connection$`, func() error {
		w.network.lookupTime = 20 * time.Millisecond
		return nil
	})
	sc.Step(`^first_byte is at least dns, connect and tls together$`, func() error {
		record, err := w.lastRecord()
		if err != nil {
			return err
		}
		sum := 0.0
		for _, phase := range []string{"dns", "connect", "tls"} {
			v, _ := record["hotline.timing."+phase].(float64)
			sum += v
		}
		if firstByte, _ := record["hotline.timing.first_byte"].(float64); firstByte < sum || sum < 0.02 {
			return failf("first_byte is %v, want at least dns, connect and tls together, %v", firstByte, sum)
		}
		return nil
	})
	sc.Step(`^a call ends$`, func() error { return w.defaultCall("Content-Type: application/json") })
	sc.Step(`^the call ends$`, func() error {
		if w.resp != nil || w.respErr != nil {
			return nil //nolint:nilerr // the call already ended; a broken reply is what some scenarios check
		}
		if w.pending != nil {
			return w.send(*w.pending)
		}
		return w.defaultCall()
	})
	sc.Step(`^the record has hotline.timing.dns, connect, tls, first_byte and total$`, func() error {
		return w.lastHasAll("hotline.timing.dns", "hotline.timing.connect", "hotline.timing.tls",
			"hotline.timing.first_byte", "hotline.timing.total")
	})
	sc.Step(`^network.peer.address holds the vendor IP$`, func() error {
		return w.lastHas("network.peer.address", w.vendor("api.talon.one").ip.String())
	})
	sc.Step(`^dns.question.name holds the name looked up$`, func() error {
		return w.lastHas("dns.question.name", "api.talon.one")
	})
	sc.Step(`^tls.protocol.version holds the vendor's TLS version$`, func() error {
		return w.lastHas("tls.protocol.version", "1.3")
	})
	sc.Step(`^tls.server.not_after holds when the vendor's certificate expires$`, func() error {
		return w.lastHas("tls.server.not_after", w.vendor("api.talon.one").notAfter.UTC().Format(time.RFC3339))
	})
	sc.Step(`^network.protocol.version holds the HTTP version used to the vendor$`, func() error {
		return w.lastHas("network.protocol.version", "2")
	})
	sc.Step(`^hotline.timing.proxy_request and proxy_response hold the proxy's own added time$`, func() error {
		record, err := w.lastRecord()
		if err != nil {
			return err
		}
		total, _ := record["hotline.timing.total"].(float64)
		for _, key := range []string{"hotline.timing.proxy_request", "hotline.timing.proxy_response"} {
			if v, ok := record[key].(float64); !ok || v < 0 || v > total {
				return failf("%s is %v, want between 0 and the total %v", key, record[key], total)
			}
		}
		return nil
	})
	sc.Step(`^hotline.connection.reused is (true|false)$`, func(reused string) error {
		return w.lastHas("hotline.connection.reused", reused)
	})

	sc.Step(`^the last byte of the reply has gone to the app$`, func() error {
		w.vendor("api.talon.one").setAnswer(func(rw http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(rw, "first part ")
			rw.(http.Flusher).Flush()
			time.Sleep(150 * time.Millisecond)
			_, _ = io.WriteString(rw, "last part")
		})
		started := time.Now()
		err := w.defaultCall()
		w.appTook = time.Since(started)
		return err
	})
	sc.Step(`^total ends at that moment$`, func() error {
		record, err := w.record()
		if err != nil {
			return err
		}
		total, _ := record["hotline.timing.total"].(float64)
		firstByte, _ := record["hotline.timing.first_byte"].(float64)
		if total < firstByte+0.14 || total > w.appTook.Seconds() {
			return failf("total is %v with first_byte %v, want it after the last part and within the app's %v", total, firstByte, w.appTook)
		}
		return nil
	})

	sc.Step(`^a vendor reached over plain http$`, func() error {
		w.addIntegration(integration("talon-shop-a", w.talonAPI("        tlsOrigination: false\n")))
		w.target = "http://api.talon.one/v2/customer_sessions/sess-8812"
		return nil
	})
	sc.Step(`^there is no (\w+) timing$`, func(phase string) error {
		record, err := w.lastRecord()
		if err != nil {
			return err
		}
		return hasNo(record, "hotline.timing."+phase)
	})
	sc.Step(`^a vendor whose host is an IP address$`, func() error {
		w.appID = ""
		w.target = "http://" + w.vendor("api.talon.one").ip.String() + "/v2/x"
		return nil
	})

	sc.Step(`^a connection to the vendor that is already open$`, func() error {
		if err := w.defaultCall(); err != nil {
			return err
		}
		return w.waitForRecords(1)
	})
	sc.Step(`^a call reuses it$`, func() error { return w.defaultCall() })
	sc.Step(`^there are no dns, connect or tls timings$`, func() error {
		record, err := w.lastRecord()
		if err != nil {
			return err
		}
		return hasNo(record, "hotline.timing.dns", "hotline.timing.connect", "hotline.timing.tls")
	})
	sc.Step(`^network.peer.address is still recorded$`, func() error {
		return w.lastHas("network.peer.address", w.vendor("api.talon.one").ip.String())
	})

	sc.Step(`^a call where (DNS fails|the TLS handshake fails|the vendor never answers)$`, func(failure string) error {
		return w.situation(map[string]string{
			"DNS fails":                "a vendor host that does not exist",
			"the TLS handshake fails":  "a TLS handshake that fails for a reason other than the certificate",
			"the vendor never answers": "a vendor slower than vendorTimeout",
		}[failure])
	})
	sc.Step(`^the record has the dns timing up to the failure and the total$`, func() error {
		return w.lastHasAll("hotline.timing.dns", "hotline.timing.total")
	})
	sc.Step(`^the record has the dns and connect timings and the total$`, func() error {
		return w.lastHasAll("hotline.timing.dns", "hotline.timing.connect", "hotline.timing.total")
	})
	sc.Step(`^the record has no first_byte timing$`, func() error {
		record, err := w.lastRecord()
		if err != nil {
			return err
		}
		return hasNo(record, "hotline.timing.first_byte")
	})

	sc.Step(`^a vendor whose IPv6 address cannot be reached and whose IPv4 address can$`, func() error {
		v := w.vendor("api.talon.one")
		v6 := netip.MustParseAddr("2001:db8::10")
		w.network.mu.Lock()
		w.network.names["api.talon.one"] = []netip.Addr{v6, v.ip}
		w.network.mu.Unlock()
		w.network.setUnreachable("[2001:db8::10]:443")
		return nil
	})
	sc.Step(`^the call goes to the IPv4 address$`, func() error {
		return w.lastHas("network.peer.address", w.vendor("api.talon.one").ip.String())
	})
	sc.Step(`^the connect phase is counted once, from the first try until connected$`, func() error {
		record, err := w.lastRecord()
		if err != nil {
			return err
		}
		connect, _ := record["hotline.timing.connect"].(float64)
		total, _ := record["hotline.timing.total"].(float64)
		if connect <= 0 || connect > total {
			return failf("connect is %v, want above 0 and within the total %v", connect, total)
		}
		return nil
	})
}

func (w *world) lastRecord() (map[string]any, error) {
	records := w.records()
	if len(records) == 0 {
		return nil, failf("no record arrived; the app got %v %q", w.resp, w.respBody)
	}
	return records[len(records)-1].attrs, nil
}

func (w *world) lastHas(key string, value any) error {
	record, err := w.lastRecord()
	if err != nil {
		return err
	}
	return has(record, key, value)
}

func (w *world) lastHasAll(keys ...string) error {
	record, err := w.lastRecord()
	if err != nil {
		return err
	}
	return hasAll(record, keys...)
}
