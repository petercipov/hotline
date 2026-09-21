// window can be emitted synchronously rather than waited on.
//
//nolint:testpackage // the specs drive the unexported flush directly, so that a
package latencies

import (
	"context"
	"math"
	"testing"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/connector"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func TestFactoryCreateDefaultConfig(t *testing.T) {
	cfg := NewFactory().CreateDefaultConfig()
	if err := componenttest.CheckConfigStruct(cfg); err != nil {
		t.Fatalf("CheckConfigStruct returned error: %v", err)
	}

	latenciesCfg, ok := cfg.(*Config)
	if !ok {
		t.Fatalf("expected *Config, got %T", cfg)
	}
	if err := latenciesCfg.Validate(); err != nil {
		t.Fatalf("default config should be valid, got: %v", err)
	}
	if latenciesCfg.Interval != defaultInterval {
		t.Fatalf("expected default interval %s, got %s", defaultInterval, latenciesCfg.Interval)
	}
	if len(latenciesCfg.Percentiles) != 3 {
		t.Fatalf("expected 3 default percentiles, got %v", latenciesCfg.Percentiles)
	}
}

func TestConfigValidate(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*Config)
		wantErr bool
	}{
		{"default", func(*Config) {}, false},
		{"zero interval", func(c *Config) { c.Interval = 0 }, true},
		{"no percentiles", func(c *Config) { c.Percentiles = nil }, true},
		{"percentile zero", func(c *Config) { c.Percentiles = []float64{0} }, true},
		{"percentile one", func(c *Config) { c.Percentiles = []float64{1} }, true},
		{"empty integration attr", func(c *Config) { c.IntegrationIDAttribute = "" }, true},
		{"empty route attr", func(c *Config) { c.RouteAttribute = "" }, true},
		{"empty method attr", func(c *Config) { c.MethodAttribute = "" }, true},
		{"empty metric name", func(c *Config) { c.MetricName = "" }, true},
		{"no span kinds", func(c *Config) { c.SpanKinds = nil }, true},
		{"unknown span kind", func(c *Config) { c.SpanKinds = []string{"banana"} }, true},
		{"sub second window", func(c *Config) { c.Window = 500 * time.Millisecond }, true},
		{"fractional window", func(c *Config) { c.Window = 1500 * time.Millisecond }, true},
		{"negative lateness", func(c *Config) { c.MaxLateness = -time.Second }, true},
		{"fractional lateness", func(c *Config) { c.MaxLateness = 1500 * time.Millisecond }, true},
		{"zero lateness", func(c *Config) { c.MaxLateness = 0 }, false},
		{"minute window", func(c *Config) { c.Window = time.Minute }, false},
		{"valid subset of span kinds", func(c *Config) { c.SpanKinds = []string{"server", "client"} }, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := createDefaultConfig().(*Config)
			tc.mutate(cfg)
			err := cfg.Validate()
			if tc.wantErr && err == nil {
				t.Fatal("expected validation error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected no validation error, got %v", err)
			}
		})
	}
}

func TestConnectorEmitsPercentilesPerSeries(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	cfg.Percentiles = []float64{0.99}
	cfg.MetricName = "custom.latency"
	sink := &metricsSink{}
	conn := newLatenciesConnector(newConnectorSettings(), cfg, sink)

	td := ptrace.NewTraces()
	addServerSpan(td, "integration-a", "/v1/orders", "GET", 0, 100*time.Millisecond)
	addServerSpan(td, "integration-a", "/v1/orders", "GET", 0, 200*time.Millisecond)
	addServerSpan(td, "integration-b", "/v1/users", "GET", 0, 50*time.Millisecond)

	if err := conn.ConsumeTraces(context.Background(), td); err != nil {
		t.Fatalf("ConsumeTraces returned error: %v", err)
	}
	if err := conn.flush(context.Background(), flushAt(time.Second)); err != nil {
		t.Fatalf("flush returned error: %v", err)
	}

	if len(sink.batches) != 1 {
		t.Fatalf("expected 1 metrics batch, got %d", len(sink.batches))
	}
	if name := metricNameOf(sink.batches[0]); name != "custom.latency" {
		t.Fatalf("expected configured metric name custom.latency, got %s", name)
	}
	dps := allDataPoints(sink.batches[0])
	// two series, one percentile each
	if len(dps) != 2 {
		t.Fatalf("expected 2 data points, got %d", len(dps))
	}
	for _, dp := range dps {
		q, _ := dp.Attributes().Get(quantileAttribute)
		if q.AsString() != "0.99" {
			t.Fatalf("expected quantile attribute 0.99, got %s", q.AsString())
		}
		if _, ok := dp.Attributes().Get(integrationIDAttribute); !ok {
			t.Fatalf("expected %s attribute", integrationIDAttribute)
		}
		if _, ok := dp.Attributes().Get(routeAttribute); !ok {
			t.Fatalf("expected %s attribute", routeAttribute)
		}
		if _, ok := dp.Attributes().Get(methodAttribute); !ok {
			t.Fatalf("expected %s attribute", methodAttribute)
		}
		if _, ok := dp.Attributes().Get(kindAttribute); !ok {
			t.Fatalf("expected %s attribute", kindAttribute)
		}
		if dp.DoubleValue() <= 0 {
			t.Fatalf("expected positive latency, got %v", dp.DoubleValue())
		}
	}
}

func TestConnectorSeparatesSeriesByMethod(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	cfg.Percentiles = []float64{0.99}
	sink := &metricsSink{}
	conn := newLatenciesConnector(newConnectorSettings(), cfg, sink)

	td := ptrace.NewTraces()
	addServerSpan(td, "integration-a", "/v1/orders", "GET", 0, 100*time.Millisecond)
	addServerSpan(td, "integration-a", "/v1/orders", "POST", 0, 100*time.Millisecond)

	if err := conn.ConsumeTraces(context.Background(), td); err != nil {
		t.Fatalf("ConsumeTraces returned error: %v", err)
	}
	if err := conn.flush(context.Background(), flushAt(time.Second)); err != nil {
		t.Fatalf("flush returned error: %v", err)
	}

	dps := allDataPoints(sink.batches[0])
	if len(dps) != 2 {
		t.Fatalf("expected 2 data points (one per method), got %d", len(dps))
	}
	methods := map[string]bool{}
	for _, dp := range dps {
		m, _ := dp.Attributes().Get(methodAttribute)
		methods[m.AsString()] = true
	}
	if !methods["GET"] || !methods["POST"] {
		t.Fatalf("expected GET and POST series, got %v", methods)
	}
}

func TestConnectorMeasuresAllSpanKindsByDefault(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	cfg.Percentiles = []float64{0.99}
	sink := &metricsSink{}
	conn := newLatenciesConnector(newConnectorSettings(), cfg, sink)

	td := ptrace.NewTraces()
	kinds := []ptrace.SpanKind{
		ptrace.SpanKindInternal,
		ptrace.SpanKindServer,
		ptrace.SpanKindClient,
		ptrace.SpanKindProducer,
		ptrace.SpanKindConsumer,
	}
	for _, k := range kinds {
		addSpan(td, k, "integration-a", "/v1/orders", "GET", 0, 100*time.Millisecond)
	}

	if err := conn.ConsumeTraces(context.Background(), td); err != nil {
		t.Fatalf("ConsumeTraces returned error: %v", err)
	}
	if err := conn.flush(context.Background(), flushAt(time.Second)); err != nil {
		t.Fatalf("flush returned error: %v", err)
	}

	dps := allDataPoints(sink.batches[0])
	if len(dps) != len(kinds) {
		t.Fatalf("expected one series per span kind (%d), got %d", len(kinds), len(dps))
	}
	got := map[string]bool{}
	for _, dp := range dps {
		v, _ := dp.Attributes().Get(kindAttribute)
		got[v.AsString()] = true
	}
	for _, want := range []string{"internal", "server", "client", "producer", "consumer"} {
		if !got[want] {
			t.Fatalf("expected a series for span kind %q, got %v", want, got)
		}
	}
}

func TestConnectorFiltersDisabledSpanKinds(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	cfg.Percentiles = []float64{0.99}
	cfg.SpanKinds = []string{"server"}
	sink := &metricsSink{}
	conn := newLatenciesConnector(newConnectorSettings(), cfg, sink)

	td := ptrace.NewTraces()
	addServerSpan(td, "integration-a", "/v1/orders", "GET", 0, 100*time.Millisecond)
	addClientSpan(td, "integration-a", "/v1/orders", "GET", 0, 100*time.Millisecond)

	if err := conn.ConsumeTraces(context.Background(), td); err != nil {
		t.Fatalf("ConsumeTraces returned error: %v", err)
	}
	if err := conn.flush(context.Background(), flushAt(time.Second)); err != nil {
		t.Fatalf("flush returned error: %v", err)
	}

	dps := allDataPoints(sink.batches[0])
	if len(dps) != 1 {
		t.Fatalf("expected only the server series, got %d data points", len(dps))
	}
	v, _ := dps[0].Attributes().Get(kindAttribute)
	if v.AsString() != "server" {
		t.Fatalf("expected server series, got %s", v.AsString())
	}
}

func TestConnectorSkipsSpansWithoutAttributes(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	sink := &metricsSink{}
	conn := newLatenciesConnector(newConnectorSettings(), cfg, sink)

	td := ptrace.NewTraces()
	// span without method - ignored
	span := appendSpan(td)
	span.SetKind(ptrace.SpanKindClient)
	span.Attributes().PutStr(cfg.IntegrationIDAttribute, "integration-a")
	span.Attributes().PutStr(cfg.RouteAttribute, "/v1/orders")
	// span without route - ignored
	span2 := appendSpan(td)
	span2.SetKind(ptrace.SpanKindServer)
	span2.Attributes().PutStr(cfg.IntegrationIDAttribute, "integration-a")
	span2.Attributes().PutStr(cfg.MethodAttribute, "GET")
	// span without integration id - ignored
	span3 := appendSpan(td)
	span3.SetKind(ptrace.SpanKindServer)
	span3.Attributes().PutStr(cfg.RouteAttribute, "/v1/orders")
	span3.Attributes().PutStr(cfg.MethodAttribute, "GET")

	if err := conn.ConsumeTraces(context.Background(), td); err != nil {
		t.Fatalf("ConsumeTraces returned error: %v", err)
	}
	if err := conn.flush(context.Background(), flushAt(time.Second)); err != nil {
		t.Fatalf("flush returned error: %v", err)
	}

	if len(sink.batches) != 0 {
		t.Fatalf("expected no metrics emitted, got %d batches", len(sink.batches))
	}
}

func TestConnectorKeepsWindowAcrossFlushes(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	sink := &metricsSink{}
	conn := newLatenciesConnector(newConnectorSettings(), cfg, sink)

	td := ptrace.NewTraces()
	addServerSpan(td, "integration-a", "/v1/orders", "GET", 0, 100*time.Millisecond)
	if err := conn.ConsumeTraces(context.Background(), td); err != nil {
		t.Fatalf("ConsumeTraces returned error: %v", err)
	}
	if err := conn.flush(context.Background(), flushAt(time.Second)); err != nil {
		t.Fatalf("flush returned error: %v", err)
	}
	// the window slides rather than tumbling, so the span is still measured on
	// the next tick even though nothing new arrived
	if err := conn.flush(context.Background(), flushAt(2*time.Second)); err != nil {
		t.Fatalf("flush returned error: %v", err)
	}

	if len(sink.batches) != 2 {
		t.Fatalf("expected 2 batches over a sliding window, got %d", len(sink.batches))
	}
	for i, batch := range sink.batches {
		if len(allDataPoints(batch)) == 0 {
			t.Fatalf("batch %d carried no data points", i)
		}
	}
}

func TestConnectorStopsEmittingOnceWindowDrains(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	sink := &metricsSink{}
	conn := newLatenciesConnector(newConnectorSettings(), cfg, sink)

	td := ptrace.NewTraces()
	addServerSpan(td, "integration-a", "/v1/orders", "GET", 0, 100*time.Millisecond)
	if err := conn.ConsumeTraces(context.Background(), td); err != nil {
		t.Fatalf("ConsumeTraces returned error: %v", err)
	}
	// past the window and the lateness it retains, so nothing is left to report
	// and the series itself is reclaimed
	if err := conn.flush(context.Background(), flushAt(2*time.Hour)); err != nil {
		t.Fatalf("flush returned error: %v", err)
	}

	if len(sink.batches) != 0 {
		t.Fatalf("expected no batch once the window drained, got %d", len(sink.batches))
	}
	if len(conn.series) != 0 {
		t.Fatalf("expected the drained series to be reclaimed, got %d", len(conn.series))
	}
}

func TestConnectorFoldsLateSpansIntoTheirSecond(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	cfg.Percentiles = []float64{0.99}
	sink := &metricsSink{}
	conn := newLatenciesConnector(newConnectorSettings(), cfg, sink)

	fast := ptrace.NewTraces()
	addServerSpan(fast, "integration-a", "/v1/orders", "GET", 0, 100*time.Millisecond)
	if err := conn.ConsumeTraces(context.Background(), fast); err != nil {
		t.Fatalf("ConsumeTraces returned error: %v", err)
	}
	if err := conn.flush(context.Background(), flushAt(10*time.Second)); err != nil {
		t.Fatalf("flush returned error: %v", err)
	}

	// a slow span for a second that has already been reported
	late := ptrace.NewTraces()
	addServerSpan(late, "integration-a", "/v1/orders", "GET", 0, 900*time.Millisecond)
	if err := conn.ConsumeTraces(context.Background(), late); err != nil {
		t.Fatalf("ConsumeTraces returned error: %v", err)
	}
	if err := conn.flush(context.Background(), flushAt(20*time.Second)); err != nil {
		t.Fatalf("flush returned error: %v", err)
	}

	if len(sink.batches) != 2 {
		t.Fatalf("expected 2 batches, got %d", len(sink.batches))
	}
	expectRelative(t, allDataPoints(sink.batches[0])[0].DoubleValue(), 0.1)
	expectRelative(t, allDataPoints(sink.batches[1])[0].DoubleValue(), 0.9)
}

func TestConnectorDropsSpansBelowTheHorizon(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	sink := &metricsSink{}
	conn := newLatenciesConnector(newConnectorSettings(), cfg, sink)

	// moves the horizon past the spans that follow
	if err := conn.flush(context.Background(), flushAt(2*time.Hour)); err != nil {
		t.Fatalf("flush returned error: %v", err)
	}

	td := ptrace.NewTraces()
	addServerSpan(td, "integration-a", "/v1/orders", "GET", 0, 100*time.Millisecond)
	if err := conn.ConsumeTraces(context.Background(), td); err != nil {
		t.Fatalf("ConsumeTraces returned error: %v", err)
	}
	if err := conn.flush(context.Background(), flushAt(2*time.Hour+time.Second)); err != nil {
		t.Fatalf("flush returned error: %v", err)
	}

	if len(sink.batches) != 0 {
		t.Fatalf("expected an expired span to be dropped, got %d batches", len(sink.batches))
	}
	if len(conn.series) != 0 {
		t.Fatalf("expected no series to be opened for it, got %d", len(conn.series))
	}
}

func TestConnectorReportsSecondsWithinSketchError(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	cfg.Percentiles = []float64{0.99}
	sink := &metricsSink{}
	conn := newLatenciesConnector(newConnectorSettings(), cfg, sink)

	td := ptrace.NewTraces()
	// 98 fast spans and one slow one, so the slow span is the p99 under
	// nearest rank rather than one place past it
	for range 98 {
		addServerSpan(td, "integration-a", "/v1/orders", "GET", 0, 100*time.Millisecond)
	}
	addServerSpan(td, "integration-a", "/v1/orders", "GET", 0, 2*time.Second)

	if err := conn.ConsumeTraces(context.Background(), td); err != nil {
		t.Fatalf("ConsumeTraces returned error: %v", err)
	}
	// the slow span ends two seconds in, and a second is measured only once it
	// has closed
	if err := conn.flush(context.Background(), flushAt(3*time.Second)); err != nil {
		t.Fatalf("flush returned error: %v", err)
	}

	dps := allDataPoints(sink.batches[0])
	if len(dps) != 1 {
		t.Fatalf("expected 1 data point, got %d", len(dps))
	}
	// reported in seconds, within the sketch's relative error
	expectRelative(t, dps[0].DoubleValue(), 2)
}

func TestConnectorMatchesValuesToUnsortedPercentiles(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	// deliberately not ascending: the sketch reads quantiles in one pass and
	// requires them sorted, so the attribute must follow the value
	cfg.Percentiles = []float64{0.99, 0.5}
	sink := &metricsSink{}
	conn := newLatenciesConnector(newConnectorSettings(), cfg, sink)

	td := ptrace.NewTraces()
	for i := 1; i <= 100; i++ {
		addServerSpan(td, "integration-a", "/v1/orders", "GET", 0, time.Duration(i)*10*time.Millisecond)
	}
	if err := conn.ConsumeTraces(context.Background(), td); err != nil {
		t.Fatalf("ConsumeTraces returned error: %v", err)
	}
	if err := conn.flush(context.Background(), flushAt(2*time.Second)); err != nil {
		t.Fatalf("flush returned error: %v", err)
	}

	byQuantile := map[string]float64{}
	for _, dp := range allDataPoints(sink.batches[0]) {
		q, _ := dp.Attributes().Get(quantileAttribute)
		byQuantile[q.AsString()] = dp.DoubleValue()
	}
	if len(byQuantile) != 2 {
		t.Fatalf("expected one data point per percentile, got %v", byQuantile)
	}
	expectRelative(t, byQuantile["0.5"], 0.5)
	expectRelative(t, byQuantile["0.99"], 0.99)
}

func TestConnectorHonoursConfiguredWindow(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	cfg.Percentiles = []float64{0.99}
	cfg.Window = time.Minute
	sink := &metricsSink{}
	conn := newLatenciesConnector(newConnectorSettings(), cfg, sink)

	td := ptrace.NewTraces()
	addServerSpan(td, "integration-a", "/v1/orders", "GET", 0, 100*time.Millisecond)
	if err := conn.ConsumeTraces(context.Background(), td); err != nil {
		t.Fatalf("ConsumeTraces returned error: %v", err)
	}
	// inside the minute it reports, past it there is nothing left
	if err := conn.flush(context.Background(), flushAt(30*time.Second)); err != nil {
		t.Fatalf("flush returned error: %v", err)
	}
	if err := conn.flush(context.Background(), flushAt(2*time.Minute)); err != nil {
		t.Fatalf("flush returned error: %v", err)
	}

	if len(sink.batches) != 1 {
		t.Fatalf("expected only the in window flush to emit, got %d batches", len(sink.batches))
	}
}

// expectRelative asserts got is want within the sketch's relative error.
func expectRelative(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want)/want > 0.02 {
		t.Fatalf("expected %v within 2%% of %v", got, want)
	}
}

func newConnectorSettings() connector.Settings {
	return connector.Settings{
		ID:                component.MustNewID("latencies"),
		TelemetrySettings: componenttest.NewNopTelemetrySettings(),
	}
}

func addServerSpan(td ptrace.Traces, integrationID, route, method string, start, duration time.Duration) {
	addSpan(td, ptrace.SpanKindServer, integrationID, route, method, start, duration)
}

func addClientSpan(td ptrace.Traces, integrationID, route, method string, start, duration time.Duration) {
	addSpan(td, ptrace.SpanKindClient, integrationID, route, method, start, duration)
}

func addSpan(td ptrace.Traces, kind ptrace.SpanKind, integrationID, route, method string, start, duration time.Duration) {
	span := appendSpan(td)
	span.SetKind(kind)
	span.Attributes().PutStr("x-integration-id", integrationID)
	span.Attributes().PutStr("http.route", route)
	span.Attributes().PutStr("http.request.method", method)
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(flushAt(start)))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(flushAt(start + duration)))
}

// baseTime anchors the specs to a realistic wall clock: the window is keyed by
// span timestamp, and the second zero of the epoch has no window behind it.
func baseTime() time.Time {
	return time.Unix(1_700_000_000, 0)
}

func flushAt(offset time.Duration) time.Time {
	return baseTime().Add(offset)
}

func appendSpan(td ptrace.Traces) ptrace.Span {
	return td.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
}

func metricNameOf(md pmetric.Metrics) string {
	return md.ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics().At(0).Name()
}

func allDataPoints(md pmetric.Metrics) []pmetric.NumberDataPoint {
	var dps []pmetric.NumberDataPoint
	rms := md.ResourceMetrics()
	for i := range rms.Len() {
		sms := rms.At(i).ScopeMetrics()
		for j := range sms.Len() {
			ms := sms.At(j).Metrics()
			for k := range ms.Len() {
				g := ms.At(k).Gauge().DataPoints()
				for l := range g.Len() {
					dps = append(dps, g.At(l))
				}
			}
		}
	}
	return dps
}

type metricsSink struct {
	batches []pmetric.Metrics
}

func (s *metricsSink) Capabilities() consumer.Capabilities {
	return consumer.Capabilities{}
}

func (s *metricsSink) ConsumeMetrics(_ context.Context, md pmetric.Metrics) error {
	s.batches = append(s.batches, md)
	return nil
}
