package proxy

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
)

// own are the numbers the proxy keeps about itself, for the telemetry
// destination: calls in flight, how long calls take and how much of that is
// the proxy's own time, failed TLS setups with apps, and config reloads. No
// number is split by integration, vendor, host, URL or route, so they stay
// few however many integrations a proxy serves (Peter, 2026-10-31).
type own struct {
	meter    metric.Meter
	active   metric.Int64UpDownCounter
	duration metric.Float64Histogram
	overhead metric.Float64Histogram
	appTLS   metric.Int64Counter
	reloads  metric.Int64Counter
	// lastReload is 1 unless the last reload failed, restart 1 while a
	// change waits for a restart.
	lastReload atomic.Int64
	restart    atomic.Int64
}

// newOwn makes the instruments. Their names, units and ranges are fixed
// and valid, so there is no error to handle.
func newOwn(meter metric.Meter) *own {
	o := &own{meter: meter}
	o.lastReload.Store(1)
	o.active, _ = meter.Int64UpDownCounter("http.server.active_requests", metric.WithUnit("{request}"),
		metric.WithDescription("Calls from apps in flight."))
	o.duration, _ = meter.Float64Histogram("http.server.request.duration", metric.WithUnit("s"),
		metric.WithDescription("How long each call took, from when it reached the proxy until the last byte went to the app."),
		metric.WithExplicitBucketBoundaries(0.005, 0.01, 0.025, 0.05, 0.075, 0.1, 0.25, 0.5, 0.75, 1, 2.5, 5, 7.5, 10))
	o.overhead, _ = meter.Float64Histogram("hotline.proxy.overhead", metric.WithUnit("s"),
		metric.WithDescription("The proxy's own time on each call: before the vendor call starts, and after the vendor's reply ends."),
		metric.WithExplicitBucketBoundaries(0.00005, 0.0001, 0.00025, 0.0005, 0.001, 0.005, 0.025, 0.1))
	o.appTLS, _ = meter.Int64Counter("hotline.app_tls.failures", metric.WithUnit("{connection}"),
		metric.WithDescription("App connections whose TLS setup with the proxy failed."))
	o.reloads, _ = meter.Int64Counter("hotline.config.reloads", metric.WithUnit("{reload}"),
		metric.WithDescription("Config reloads, by hotline.reload.result."))
	return o
}

// observe sends what the proxy is now: the config in use, whether the last
// reload worked, whether a change waits for a restart, and when its
// certificates expire.
func (o *own) observe(p *Proxy) {
	info, _ := o.meter.Int64ObservableGauge("hotline.config.info", metric.WithUnit("1"),
		metric.WithDescription("Always 1; its labels say which config is in use and whether the proxy port takes TLS and CONNECT."))
	last, _ := o.meter.Int64ObservableGauge("hotline.config.reload.last_success", metric.WithUnit("1"),
		metric.WithDescription("1 unless the last reload failed."))
	restart, _ := o.meter.Int64ObservableGauge("hotline.config.restart_needed", metric.WithUnit("1"),
		metric.WithDescription("1 while a changed setting waits for a restart."))
	notAfter, _ := o.meter.Int64ObservableGauge("hotline.certificate.not_after", metric.WithUnit("s"),
		metric.WithDescription("When a certificate of the proxy's own expires, in seconds since 1970."))
	_, _ = o.meter.RegisterCallback(func(_ context.Context, ob metric.Observer) error {
		ob.ObserveInt64(info, 1, metric.WithAttributes(
			attribute.String("hotline.config.version", p.handler.cfg.Load().Label()),
			attribute.Bool("hotline.listen.tls", p.appCert != nil),
			attribute.Bool("hotline.listen.connect", p.handler.intercept != nil),
		))
		ob.ObserveInt64(last, o.lastReload.Load())
		ob.ObserveInt64(restart, o.restart.Load())
		if p.appCert != nil {
			ob.ObserveInt64(notAfter, p.appCert.get().Leaf.NotAfter.Unix(),
				metric.WithAttributes(attribute.String("hotline.certificate.use", "listen.tls")))
		}
		if p.handler.intercept != nil {
			ob.ObserveInt64(notAfter, p.handler.intercept.issuer.get().Leaf.NotAfter.Unix(),
				metric.WithAttributes(attribute.String("hotline.certificate.use", "listen.connect")))
		}
		return nil
	}, info, last, restart, notAfter)
}

// started counts a call in flight, until the returned func is called.
func (o *own) started(r *http.Request, ingress string) func() {
	labels := metric.WithAttributes(
		semconv.HTTPRequestMethodKey.String(knownMethod(r.Method)),
		semconv.URLScheme(appScheme(r)),
		attribute.String("hotline.ingress", ingress),
	)
	o.active.Add(context.Background(), 1, labels)
	return func() { o.active.Add(context.Background(), -1, labels) }
}

// ended measures a finished call from its record: its total time, and the
// proxy's own part of it once the call got as far as the vendor.
func (o *own) ended(r *http.Request, attrs []attribute.KeyValue) {
	labels := []attribute.KeyValue{semconv.URLScheme(appScheme(r))}
	var total, proxyUS int64
	reached := false
	var ingress attribute.KeyValue
	for _, kv := range attrs {
		switch kv.Key {
		case semconv.HTTPRequestMethodKey:
			labels = append(labels, semconv.HTTPRequestMethodKey.String(knownMethod(kv.Value.AsString())))
		case semconv.HTTPResponseStatusCodeKey, semconv.ErrorTypeKey:
			labels = append(labels, kv)
		case "hotline.ingress":
			ingress = kv
			labels = append(labels, kv)
		case "hotline.timing.total_us":
			total = kv.Value.AsInt64()
		case "hotline.timing.proxy_request_us":
			reached = true
			proxyUS += kv.Value.AsInt64()
		case "hotline.timing.proxy_response_us":
			proxyUS += kv.Value.AsInt64()
		}
	}
	ctx := context.Background()
	o.duration.Record(ctx, float64(total)/1e6, metric.WithAttributes(labels...))
	if reached {
		o.overhead.Record(ctx, float64(proxyUS)/1e6, metric.WithAttributes(ingress))
	}
}

// tlsFailed counts an app connection whose TLS setup failed, inside a
// tunnel or, with no ingress, on the TLS port.
func (o *own) tlsFailed(err error, labels ...attribute.KeyValue) {
	o.appTLS.Add(context.Background(), 1,
		metric.WithAttributes(append(labels, semconv.ErrorTypeKey.String(tlsFailure(err)))...))
}

// reloaded counts a reload, and whether a setting it changed waits for a
// restart.
func (o *own) reloaded(ok, restart bool) {
	result := "failure"
	o.lastReload.Store(0)
	if ok {
		result = "success"
		o.lastReload.Store(1)
		o.restart.Store(0)
		if restart {
			o.restart.Store(1)
		}
	}
	o.reloads.Add(context.Background(), 1, metric.WithAttributes(attribute.String("hotline.reload.result", result)))
}

// tlsFailure says why an app's TLS setup failed: plain HTTP on the TLS
// port, only TLS versions the proxy does not take, or the app not trusting
// the certificate it was shown.
func tlsFailure(err error) string {
	switch {
	case errors.Is(err, errPlainHTTP):
		return "plain_http"
	case strings.Contains(err.Error(), "unsupported versions"):
		return "tls_version"
	}
	return untrustedOr(err)
}

// untrustedOr is tls_untrusted_by_app when the app refused the certificate
// it was shown, else tls_handshake_failed.
func untrustedOr(err error) string {
	if text := err.Error(); strings.Contains(text, "remote error: tls: ") && strings.Contains(text, "certificate") {
		return "tls_untrusted_by_app"
	}
	return "tls_handshake_failed"
}

// knownMethod is the method, or _OTHER for one the standard does not
// list, so a made up method cannot add labels.
func knownMethod(method string) string {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch,
		http.MethodDelete, http.MethodConnect, http.MethodOptions, http.MethodTrace:
		return method
	}
	return "_OTHER"
}

// appScheme is https when the app set up TLS with the proxy, else http.
func appScheme(r *http.Request) string {
	if a := appConnOf(r); a != nil && a.secure {
		return "https"
	}
	return "http"
}
