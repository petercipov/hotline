package export

import (
	"cmp"
	"context"
	"crypto/tls"
	"crypto/x509"
	"strconv"
	"strings"
	"sync/atomic"

	"go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"google.golang.org/grpc/status"

	"hotline-proxy/internal/config"
)

// Own sends the numbers the proxy keeps about itself to the telemetry
// destination, and nowhere else. Without one they are not measured at all.
type Own struct {
	provider *sdkmetric.MeterProvider
	meter    metric.Meter
}

// NewOwn starts the pipeline of the proxy's own numbers, with the Go
// runtime's among them. A metric telemetry.disable names is dropped before
// it is measured. With no telemetry destination, nothing is measured.
func NewOwn(t *config.Telemetry, roots *x509.CertPool, service Service, timing Timing) *Own {
	if t == nil {
		return &Own{meter: noop.NewMeterProvider().Meter("hotline-proxy")}
	}
	options := []sdkmetric.Option{
		sdkmetric.WithResource(describe(service, t.Exporter.Resource)),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricSender(t.Exporter, roots, timing),
			sdkmetric.WithInterval(timing.Interval),
			sdkmetric.WithTimeout(timing.Timeout))),
	}
	for _, name := range t.Disable {
		options = append(options, sdkmetric.WithView(sdkmetric.NewView(
			sdkmetric.Instrument{Name: name},
			sdkmetric.Stream{Aggregation: sdkmetric.AggregationDrop{}})))
	}
	provider := sdkmetric.NewMeterProvider(options...)
	// The runtime's instruments are fixed and valid, so there is no error
	// to handle.
	_ = runtime.Start(runtime.WithMeterProvider(provider))
	return &Own{provider: provider, meter: provider.Meter("hotline-proxy")}
}

// Meter makes the instruments of the proxy's own numbers.
func (o *Own) Meter() metric.Meter {
	return o.meter
}

// Shutdown sends the last numbers, within the time ctx allows.
func (o *Own) Shutdown(ctx context.Context) error {
	if o.provider == nil {
		return nil
	}
	return o.provider.Shutdown(ctx)
}

// destinationTLS trusts roots and the destination's own issuers, and shows
// its client certificate, if any.
func destinationTLS(e config.Exporter, roots *x509.CertPool) *tls.Config {
	pool := roots.Clone()
	pool.AppendCertsFromPEM(e.CAs)
	c := &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
	if e.Certificate != nil {
		c.Certificates = []tls.Certificate{*e.Certificate}
	}
	return c
}

// sendStats are the instruments that count what each destination's queue
// and senders did, the OpenTelemetry SDK's own metrics.
type sendStats struct {
	processed      metric.Int64Counter
	logs           metric.Int64Counter
	points         metric.Int64Counter
	spansProcessed metric.Int64Counter
	spans          metric.Int64Counter
}

// newSendStats makes the counters. Their names and units are fixed and
// valid, so there is no error to handle.
func newSendStats(meter metric.Meter) sendStats {
	var s sendStats
	s.processed, _ = meter.Int64Counter("otel.sdk.processor.log.processed", metric.WithUnit("{log_record}"),
		metric.WithDescription("Records the queue handed on to be sent, or dropped with error.type queue_full."))
	s.logs, _ = meter.Int64Counter("otel.sdk.exporter.log.exported", metric.WithUnit("{log_record}"),
		metric.WithDescription("Records sent, or lost in a failed send with an error.type."))
	s.points, _ = meter.Int64Counter("otel.sdk.exporter.metric_data_point.exported", metric.WithUnit("{data_point}"),
		metric.WithDescription("Data points of hotline.requests sent, or lost in a failed send with an error.type."))
	s.spansProcessed, _ = meter.Int64Counter("otel.sdk.processor.span.processed", metric.WithUnit("{span}"),
		metric.WithDescription("Spans the queue handed on to be sent, or dropped with error.type queue_full."))
	s.spans, _ = meter.Int64Counter("otel.sdk.exporter.span.exported", metric.WithUnit("{span}"),
		metric.WithDescription("Spans sent, or lost in a failed send with an error.type."))
	return s
}

// observeDestinations sends, for every destination, how full its queue is
// and whether its last send worked.
func observeDestinations(meter metric.Meter, destinations []destination) {
	size, _ := meter.Int64ObservableUpDownCounter("otel.sdk.processor.log.queue.size", metric.WithUnit("{log_record}"),
		metric.WithDescription("Records waiting to be sent."))
	capacity, _ := meter.Int64ObservableUpDownCounter("otel.sdk.processor.log.queue.capacity", metric.WithUnit("{log_record}"),
		metric.WithDescription("Records the queue holds at most."))
	spanSize, _ := meter.Int64ObservableUpDownCounter("otel.sdk.processor.span.queue.size", metric.WithUnit("{span}"),
		metric.WithDescription("Spans waiting to be sent."))
	spanCapacity, _ := meter.Int64ObservableUpDownCounter("otel.sdk.processor.span.queue.capacity", metric.WithUnit("{span}"),
		metric.WithDescription("Spans the queue holds at most."))
	state, _ := meter.Int64ObservableGauge("hotline.destination.state", metric.WithUnit("1"),
		metric.WithDescription("1 while sends to the destination work, 0 from a failed send until one works again."))
	_, _ = meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		for _, d := range destinations {
			waiting, room := d.queue.fill()
			o.ObserveInt64(size, int64(waiting), metric.WithAttributeSet(d.queue.processor))
			o.ObserveInt64(capacity, int64(room), metric.WithAttributeSet(d.queue.processor))
			waiting, room = d.spans.fill()
			o.ObserveInt64(spanSize, int64(waiting), metric.WithAttributeSet(d.spans.processor))
			o.ObserveInt64(spanCapacity, int64(room), metric.WithAttributeSet(d.spans.processor))
			o.ObserveInt64(state, d.up.Load(), metric.WithAttributeSet(d.place))
		}
		return nil
	}, size, capacity, spanSize, spanCapacity, state)
}

// component names one part of one destination the way the SDK metrics
// do, with the destination's address.
func component(kind string, index int, e config.Exporter) attribute.Set {
	return attribute.NewSet(append(address(e),
		attribute.String("otel.component.type", kind),
		attribute.String("otel.component.name", kind+"/"+strconv.Itoa(index)))...)
}

// address is where a destination is, as server.address and server.port.
// An endpoint with no port uses its scheme's.
func address(e config.Exporter) []attribute.KeyValue {
	port, _ := strconv.Atoi(cmp.Or(e.URL.Port(), map[string]string{"http": "80", "https": "443"}[e.URL.Scheme]))
	return []attribute.KeyValue{semconv.ServerAddress(e.URL.Hostname()), semconv.ServerPort(port)}
}

// kinds are the SDK's names for the log, metric and span senders of a
// destination.
func kinds(e config.Exporter) (string, string, string) {
	if e.HTTP {
		return "otlp_http_log_exporter", "otlp_http_metric_exporter", "otlp_http_span_exporter"
	}
	return "otlp_grpc_log_exporter", "otlp_grpc_metric_exporter", "otlp_grpc_span_exporter"
}

// failed adds error.type to a set: the gRPC status, such as unavailable,
// or _OTHER when the error has none.
func failed(set attribute.Set, err error) metric.MeasurementOption {
	kind := "_OTHER"
	if s, ok := status.FromError(err); ok {
		kind = strings.ToLower(s.Code().String())
	}
	return metric.WithAttributes(append(set.ToSlice(), semconv.ErrorTypeKey.String(kind))...)
}

// countedMetrics counts the data points each send of hotline.requests
// carried, and keeps the destination's state.
type countedMetrics struct {
	sdkmetric.Exporter
	points metric.Int64Counter
	set    attribute.Set
	up     *atomic.Int64
}

func (c countedMetrics) Export(ctx context.Context, rm *metricdata.ResourceMetrics) error {
	err := c.Exporter.Export(ctx, rm)
	n := 0
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			// Destinations get only the hotline.requests sum.
			sum, _ := m.Data.(metricdata.Sum[int64])
			n += len(sum.DataPoints)
		}
	}
	measured(ctx, c.points, n, c.set, err, c.up)
	return err
}

// measured counts n sent or lost, and keeps the state of the destination.
func measured(ctx context.Context, counter metric.Int64Counter, n int, set attribute.Set, err error, up *atomic.Int64) {
	if err != nil {
		up.Store(0)
		counter.Add(ctx, int64(n), failed(set, err))
		return
	}
	up.Store(1)
	counter.Add(ctx, int64(n), metric.WithAttributeSet(set))
}
