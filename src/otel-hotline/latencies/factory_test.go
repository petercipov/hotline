package latencies

import (
	"context"
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
	if err := conn.flush(context.Background(), time.Unix(0, 0)); err != nil {
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
	if err := conn.flush(context.Background(), time.Unix(0, 0)); err != nil {
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
	if err := conn.flush(context.Background(), time.Unix(0, 0)); err != nil {
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
	if err := conn.flush(context.Background(), time.Unix(0, 0)); err != nil {
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
	if err := conn.flush(context.Background(), time.Unix(0, 0)); err != nil {
		t.Fatalf("flush returned error: %v", err)
	}

	if len(sink.batches) != 0 {
		t.Fatalf("expected no metrics emitted, got %d batches", len(sink.batches))
	}
}

func TestConnectorFlushResetsAccumulators(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	sink := &metricsSink{}
	conn := newLatenciesConnector(newConnectorSettings(), cfg, sink)

	td := ptrace.NewTraces()
	addServerSpan(td, "integration-a", "/v1/orders", "GET", 0, 100*time.Millisecond)
	if err := conn.ConsumeTraces(context.Background(), td); err != nil {
		t.Fatalf("ConsumeTraces returned error: %v", err)
	}
	if err := conn.flush(context.Background(), time.Unix(0, 0)); err != nil {
		t.Fatalf("flush returned error: %v", err)
	}
	// second flush with no new data must not emit anything
	if err := conn.flush(context.Background(), time.Unix(0, 0)); err != nil {
		t.Fatalf("flush returned error: %v", err)
	}
	if len(sink.batches) != 1 {
		t.Fatalf("expected exactly 1 batch after reset, got %d", len(sink.batches))
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
	span.SetStartTimestamp(pcommon.Timestamp(start))
	span.SetEndTimestamp(pcommon.Timestamp(start + duration))
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
	for i := 0; i < rms.Len(); i++ {
		sms := rms.At(i).ScopeMetrics()
		for j := 0; j < sms.Len(); j++ {
			ms := sms.At(j).Metrics()
			for k := 0; k < ms.Len(); k++ {
				g := ms.At(k).Gauge().DataPoints()
				for l := 0; l < g.Len(); l++ {
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
