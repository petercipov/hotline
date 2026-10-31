package proxy_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

func telemetrySteps(sc *godog.ScenarioContext, w *world) {
	sc.Step(`^a fake OTLP receiver for the proxy's own telemetry$`, func() error {
		w.telemetry = newReceiver()
		return nil
	})
	sc.Step(`^a config with no telemetry section$`, func() error {
		w.telemetry.stop()
		w.telemetry = nil
		return nil
	})
	sc.Step(`^the telemetry section turns off (\S+)$`, func(name string) error {
		w.telemetryDisable = append(w.telemetryDisable, name)
		return nil
	})

	sc.Step(`^the telemetry receiver gets the (?:metric|histogram) (\S+)$`, func(name string) error {
		_, err := w.ownPoints(name)
		return err
	})
	sc.Step(`^the telemetry receiver gets the metrics (\S+) and (\S+)$`, func(a, b string) error {
		if _, err := w.ownPoints(a); err != nil {
			return err
		}
		_, err := w.ownPoints(b)
		return err
	})
	sc.Step(`^the telemetry receiver gets the histogram (\S+) with unit (\S+)$`, func(name, unit string) error {
		points, err := w.ownPoints(name)
		if err != nil {
			return err
		}
		if points[0].kind != "histogram" || points[0].unit != unit {
			return failf("%s is a %s with unit %q", name, points[0].kind, points[0].unit)
		}
		return nil
	})
	sc.Step(`^the telemetry receiver gets no log records$`, func() error {
		w.settle()
		if records, _ := w.telemetry.got(); len(records) > 0 {
			return failf("the telemetry receiver got %d records", len(records))
		}
		return nil
	})
	sc.Step(`^the product receiver gets no metric but hotline.requests$`, func() error {
		w.settle()
		for _, p := range w.receivers[0].metricPoints() {
			if p.name != "hotline.requests" {
				return failf("the product receiver got %s", p.name)
			}
		}
		return nil
	})
	sc.Step(`^the telemetry receiver gets no metric (\S+)$`, func(name string) error {
		return w.noOwnMetric(func(got string) bool { return got == name })
	})
	sc.Step(`^the telemetry receiver gets no metric starting with (\S+)$`, func(prefix string) error {
		return w.noOwnMetric(func(got string) bool { return strings.HasPrefix(got, prefix) })
	})

	sc.Step(`^a vendor that keeps the call waiting$`, func() error {
		w.freed = make(chan struct{})
		freed := w.freed
		w.vendor("api.talon.one").setAnswer(func(rw http.ResponseWriter, _ *http.Request) {
			<-freed
			rw.WriteHeader(http.StatusOK)
		})
		return nil
	})
	sc.Step(`^an app starts a forward proxy GET call over http$`, func() error {
		if err := w.start(); err != nil {
			return err
		}
		done := make(chan error, 1)
		w.callDone = done
		go func() { done <- w.call(http.MethodGet, "http://api.talon.one/v2/customer_sessions/sess-8812") }()
		return w.waitForVendorCalls("api.talon.one", 1)
	})
	sc.Step(`^once the call ends it is 0$`, func() error {
		close(w.freed)
		if err := <-w.callDone; err != nil {
			return err
		}
		return w.ownValue("http.server.active_requests", "http.request.method GET, url.scheme http and hotline.ingress forward_proxy", 0)
	})
	sc.Step(`^an app makes a forward proxy GET call over http(?: and the vendor answers 200)?$`, func() error {
		return w.call(http.MethodGet, "http://api.talon.one/v2/customer_sessions/sess-8812")
	})
	sc.Step(`^the vendor's name does not resolve$`, func() error {
		w.missing["api.talon.one"] = true
		return nil
	})
	sc.Step(`^an app makes a forward proxy call with the method (\S+)$`, func(method string) error {
		return w.call(method, "http://api.talon.one/v2/customer_sessions/sess-8812")
	})

	sc.Step(`^(\S+) (?:is|turns) (\d+) for (.+)$`, func(name string, want int, labels string) error {
		if labels == "that destination" {
			labels = w.destinationLabels()
		}
		return w.ownValue(name, labels, float64(want))
	})
	sc.Step(`^(\S+) counts (\d+) calls? (?:for|with) (.+)$`, func(name string, want uint64, labels string) error {
		if name == "it" {
			name = w.lastMetric
		}
		return w.ownCount(name, labels, want)
	})
	sc.Step(`^its range limits are (.+)$`, func(list string) error {
		points, err := w.ownPoints(w.lastMetric)
		if err != nil {
			return err
		}
		var want []float64
		for _, item := range splitList(list) {
			f, _ := strconv.ParseFloat(item, 64)
			want = append(want, f)
		}
		if !slices.Equal(points[0].bounds, want) {
			return failf("%s has the range limits %v, want %v", w.lastMetric, points[0].bounds, want)
		}
		return nil
	})
	sc.Step(`^it has no label but (.+)$`, func(list string) error {
		allowed := splitList(list)
		points, err := w.ownPoints(w.lastMetric)
		if err != nil {
			return err
		}
		for _, p := range points {
			for key := range p.attrs {
				if !slices.Contains(allowed, key) {
					return failf("%s has the label %s", w.lastMetric, key)
				}
			}
		}
		return nil
	})
	sc.Step(`^(\S+) has no numbers for (.+)$`, func(name, labels string) error {
		points, err := w.ownPoints(name)
		if err != nil {
			return err
		}
		for _, p := range points {
			if matches(p, parseLabels(labels)) {
				return failf("%s has numbers for %s", name, labels)
			}
		}
		return nil
	})

	sc.Step(`^a product destination with queueSize (\d+)$`, func(size int) error {
		w.exporterExtra = []string{fmt.Sprintf("      queueSize: %d\n", size)}
		return nil
	})
	sc.Step(`^(\S+) is sent for that destination$`, func(name string) error {
		return w.eventually(func() error {
			if _, ok := w.latest(name, parseLabels(w.destinationLabels())); !ok {
				return failf("no %s for that destination", name)
			}
			return nil
		})
	})
	sc.Step(`^(\S+) counts (\d+) records? with (no|an) error.type (?:(\S+) )?for that destination$`,
		func(name string, want int, which, errorType string) error {
			return w.eventually(func() error {
				var got float64
				for _, p := range w.lastPoints(name) {
					_, has := p.attrs["error.type"]
					if !matches(p, parseLabels(w.destinationLabels())) || has != (which == "an") ||
						errorType != "" && p.attrs["error.type"] != errorType {
						continue
					}
					got += p.value
				}
				if got != float64(want) {
					return failf("%s counts %v, want %d", name, got, want)
				}
				return nil
			})
		})
	sc.Step(`^otel.sdk.processor.log.processed counts (\d+) record with error.type queue_full for that destination$`, func(want int) error {
		return w.ownValue("otel.sdk.processor.log.processed", w.processorLabels()+" and error.type queue_full", float64(want))
	})
	sc.Step(`^(\S+) counts data points with no error.type for that destination$`, func(name string) error {
		return w.eventually(func() error {
			for _, p := range w.lastPoints(name) {
				if _, has := p.attrs["error.type"]; matches(p, parseLabels(w.destinationLabels())) && !has && p.value > 0 {
					return nil
				}
			}
			return failf("%s counted no data points", name)
		})
	})
	sc.Step(`^3 calls end$`, func() error {
		for range 3 {
			if err := w.defaultCall(); err != nil {
				return err
			}
		}
		return nil
	})
	sc.Step(`^a product destination over HTTP that refuses every log export with 400$`, func() error {
		w.receivers[0].stop()
		w.receivers[0] = newReceiverWith(receiverSetup{http: true})
		for range 100 {
			w.receivers[0].answers = append(w.receivers[0].answers, answer{status: http.StatusBadRequest})
		}
		return nil
	})
	sc.Step(`^a product destination that is down$`, func() error {
		w.receivers[0].stop()
		w.receivers[0] = &receiver{endpoint: freePort()}
		return nil
	})
	sc.Step(`^the destination comes back and another call ends$`, func() error {
		l, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", w.receivers[0].endpoint)
		if err != nil {
			return err
		}
		w.receivers[0] = newReceiverWith(receiverSetup{listener: l})
		return w.defaultCall()
	})

	sc.Step(`^hotline.config.info is 1 with the config's hotline.config.version$`, func() error {
		return w.ownValue("hotline.config.info", "hotline.config.version "+w.configVersion("hotline proxy started"), 1)
	})
	sc.Step(`^it says hotline.listen.tls (true|false) and hotline.listen.connect (true|false)$`, func(tls, connect string) error {
		return w.ownValue("hotline.config.info", "hotline.listen.tls "+tls+" and hotline.listen.connect "+connect, 1)
	})
	sc.Step(`^hotline.config.info says hotline.listen.tls (true|false)$`, func(tls string) error {
		return w.ownValue("hotline.config.info", "hotline.listen.tls "+tls, 1)
	})
	sc.Step(`^hotline.config.info still names the first config version$`, func() error {
		return w.ownValue("hotline.config.info", "hotline.config.version "+w.configVersion("hotline proxy started"), 1)
	})
	sc.Step(`^hotline.config.info names the new config version$`, func() error {
		return w.ownValue("hotline.config.info", "hotline.config.version "+w.configVersion("config reloaded"), 1)
	})
	sc.Step(`^(hotline\.config\.\S+) (?:is|turns) (\d+)$`, func(name string, want int) error {
		return w.ownValue(name, "", float64(want))
	})
	sc.Step(`^hotline.config.reloads counts (\d+) with hotline.reload.result (\S+)$`, func(want int, result string) error {
		return w.ownValue("hotline.config.reloads", "hotline.reload.result "+result, float64(want))
	})
	sc.Step(`^the config file is put back as it was$`, func() error {
		w.writeFile("hotline-proxy.yaml", w.config())
		return nil
	})

	sc.Step(`^the proxy port takes TLS(?: with a certificate that expires in (\d+) days)?$`, func(days string) error {
		n, _ := strconv.Atoi(days)
		w.proxyCert = w.issuer.leaf("hotline-proxy", time.Now().Add(time.Duration(max(n, 90))*24*time.Hour), false)
		w.writeKeyPair(w.proxyCert, w.proxyCert)
		w.listenTLS = "  tls: { cert: ./tls/cert.pem, key: ./tls/key.pem }\n"
		w.appTLS = w.appTLSConfig()
		return nil
	})
	sc.Step(`^the cert and key files are replaced with a certificate that expires in (\d+) days$`, func(days int) error {
		renewed := w.issuer.leaf("hotline-proxy", time.Now().Add(time.Duration(days)*24*time.Hour), false)
		w.renewed = &renewed
		w.replaceKeyPair(renewed, renewed)
		return nil
	})
	sc.Step(`^hotline.certificate.not_after is that certificate's expiry for hotline.certificate.use listen.tls$`, func() error {
		return w.ownValue("hotline.certificate.not_after", "hotline.certificate.use listen.tls", expiry(w.proxyCert))
	})
	sc.Step(`^hotline.certificate.not_after turns to the new certificate's expiry for hotline.certificate.use listen.tls$`, func() error {
		return w.ownValue("hotline.certificate.not_after", "hotline.certificate.use listen.tls", expiry(*w.renewed))
	})
	sc.Step(`^CONNECT is an ingress with an issuer that expires in (\d+) days$`, func(days int) error {
		w.useInterceptionIssuer(newIssuerValid(time.Now().Add(-time.Hour), time.Now().Add(time.Duration(days)*24*time.Hour)))
		return nil
	})
	sc.Step(`^hotline.certificate.not_after is that issuer's expiry for hotline.certificate.use listen.connect$`, func() error {
		return w.ownValue("hotline.certificate.not_after", "hotline.certificate.use listen.connect", float64(w.interception.cert.NotAfter.Unix()))
	})

	sc.Step(`^an app that does not trust the proxy opens TLS$`, func() error {
		w.appTLS = &tls.Config{RootCAs: x509.NewCertPool(), ServerName: "hotline-proxy", MinVersion: tls.VersionTLS12}
		return w.tryTLS()
	})
	sc.Step(`^an app that allows only TLS 1.1 opens TLS$`, func() error {
		w.appTLS.MinVersion, w.appTLS.MaxVersion = tls.VersionTLS10, tls.VersionTLS11
		return w.tryTLS()
	})
	sc.Step(`^an app sends plain HTTP to the TLS port$`, func() error {
		w.appTLS = nil
		return w.defaultCall()
	})
	sc.Step(`^an app that does not trust the issuer opens a tunnel$`, func() error {
		w.useInterceptionIssuer(newIssuer())
		w.tunnelRoots = x509.NewCertPool()
		return w.openTunnel("api.talon.one", "443", "talon-shop-a")
	})
	sc.Step(`^hotline.app_tls.failures counts (\d+) with error.type (\S+) and hotline.ingress (\S+)$`, func(want int, errorType, ingress string) error {
		labels := "error.type " + errorType
		if ingress != "none" {
			labels += " and hotline.ingress " + ingress
		}
		return w.eventually(func() error {
			p, ok := w.latest("hotline.app_tls.failures", parseLabels(labels))
			_, hasIngress := p.attrs["hotline.ingress"]
			if !ok || p.value != float64(want) || hasIngress != (ingress != "none") {
				return failf("hotline.app_tls.failures is %v for %s", p.value, labels)
			}
			return nil
		})
	})

	sc.Step(`^the proxy logs a warning naming (\S+)$`, func(name string) error {
		if err := w.start(); err != nil {
			return err
		}
		return w.warned(name)
	})
	sc.Step(`^the proxy logs a warning that its own telemetry is not sent$`, func() error {
		return w.warned("telemetry is not sent")
	})
}

// ownPoints waits until the telemetry receiver has data points of a metric,
// and returns the latest of each set of labels.
func (w *world) ownPoints(name string) ([]receivedPoint, error) {
	w.lastMetric = name
	if err := w.start(); err != nil {
		return nil, err
	}
	var points []receivedPoint
	err := w.eventually(func() error {
		points = w.lastPoints(name)
		if len(points) == 0 {
			return failf("the telemetry receiver got no %s", name)
		}
		return nil
	})
	return points, err
}

// lastPoints are the latest data points of a metric, one for each set of
// labels, as the telemetry receiver got them.
func (w *world) lastPoints(name string) []receivedPoint {
	latest := map[string]receivedPoint{}
	var order []string
	for _, p := range w.telemetry.metricPoints() {
		if p.name != name {
			continue
		}
		key := fmt.Sprint(p.attrs)
		if _, seen := latest[key]; !seen {
			order = append(order, key)
		}
		latest[key] = p
	}
	var points []receivedPoint
	for _, key := range order {
		points = append(points, latest[key])
	}
	return points
}

// latest is the latest data point of a metric with every label given.
func (w *world) latest(name string, labels map[string]string) (receivedPoint, bool) {
	for _, p := range w.lastPoints(name) {
		if matches(p, labels) {
			return p, true
		}
	}
	return receivedPoint{}, false
}

// ownValue waits until a metric's latest value with the labels is want.
func (w *world) ownValue(name, labels string, want float64) error {
	if err := w.start(); err != nil {
		return err
	}
	return w.eventually(func() error {
		p, ok := w.latest(name, parseLabels(labels))
		if !ok || p.value != want {
			return failf("%s is %v for %q, want %v; got %v", name, p.value, labels, want, w.lastPoints(name))
		}
		return nil
	})
}

// ownCount waits until a histogram has counted want calls with the labels.
func (w *world) ownCount(name, labels string, want uint64) error {
	w.lastMetric = name
	return w.eventually(func() error {
		p, ok := w.latest(name, parseLabels(labels))
		if !ok || p.count != want {
			return failf("%s counts %d for %q, want %d; got %v", name, p.count, labels, want, w.lastPoints(name))
		}
		return nil
	})
}

// noOwnMetric checks that no metric the test names arrived, once the
// proxy's own numbers have been sent at least once.
func (w *world) noOwnMetric(unwanted func(string) bool) error {
	if _, err := w.ownPoints("hotline.config.info"); err != nil {
		return err
	}
	for _, p := range w.telemetry.metricPoints() {
		if unwanted(p.name) {
			return failf("the telemetry receiver got %s", p.name)
		}
	}
	return nil
}

// eventually tries a check until it passes, for up to 10 seconds.
func (w *world) eventually(check func() error) error {
	deadline := time.Now().Add(10 * time.Second)
	for {
		err := check()
		if err == nil || time.Now().After(deadline) {
			return err
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// destinationLabels name the first product destination by its address.
func (w *world) destinationLabels() string {
	host, port, _ := net.SplitHostPort(strings.TrimPrefix(w.receivers[0].endpoint, "http://"))
	return "server.address " + host + " and server.port " + port
}

// processorLabels name the queue of the first product destination.
func (w *world) processorLabels() string {
	return w.destinationLabels() + " and otel.component.type batching_log_processor"
}

// configVersion is the config version the log line with message names.
func (w *world) configVersion(message string) string {
	version := regexp.MustCompile(`msg="?` + message + `"? .*config_version=(\S+)`)
	var found string
	for _, line := range strings.Split(w.logs.String(), "\n") {
		if m := version.FindStringSubmatch(line); m != nil {
			found = m[1]
		}
	}
	return found
}

// tryTLS opens TLS to the proxy, which may fail.
func (w *world) tryTLS() error {
	if err := w.start(); err != nil {
		return err
	}
	conn, err := w.dialProxy()
	if err == nil {
		_ = conn.Close()
	}
	return nil
}

func expiry(cert tls.Certificate) float64 {
	parsed, _ := x509.ParseCertificate(cert.Certificate[0])
	return float64(parsed.NotAfter.Unix())
}

// splitList splits "a, b and c" into its items.
func splitList(list string) []string {
	return strings.FieldsFunc(strings.ReplaceAll(list, " and ", ", "), func(r rune) bool { return r == ',' || r == ' ' })
}

// parseLabels reads "name value, name value and name value".
func parseLabels(labels string) map[string]string {
	parsed := map[string]string{}
	for _, pair := range strings.Split(strings.ReplaceAll(labels, " and ", ", "), ", ") {
		if name, value, ok := strings.Cut(strings.TrimSpace(pair), " "); ok {
			parsed[name] = value
		}
	}
	return parsed
}

// matches says whether a data point has every label given.
func matches(p receivedPoint, labels map[string]string) bool {
	for name, value := range labels {
		got, ok := p.attrs[name]
		if !ok || fmt.Sprint(got) != value {
			return false
		}
	}
	return true
}
