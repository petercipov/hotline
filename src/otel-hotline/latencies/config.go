package latencies

import (
	"fmt"
	"time"

	"go.opentelemetry.io/collector/component"
)

var Type = component.MustNewType("latencies")

const (
	defaultIntegrationIDAttribute = "x-integration-id"
	defaultRouteAttribute         = "http.route"
	defaultMethodAttribute        = "http.request.method"
	defaultInterval               = 10 * time.Second
	defaultMetricName             = "http.span.request.duration"
)

func defaultPercentiles() []float64 {
	return []float64{0.99, 0.8, 0.75}
}

// Config configures the latencies connector that turns HTTP server span
// durations into latency percentile metrics per integration id and route.
type Config struct {
	// Percentiles is the set of quantiles to compute, each in the open
	// interval (0, 1). Defaults to p99, p80, p75.
	Percentiles []float64 `mapstructure:"percentiles"`
	// Interval is how often percentile metrics are computed and emitted.
	Interval time.Duration `mapstructure:"interval"`
	// IntegrationIDAttribute is the span attribute key carrying the
	// integration id used to partition metrics.
	IntegrationIDAttribute string `mapstructure:"integration_id_attribute"`
	// RouteAttribute is the span attribute key carrying the HTTP route.
	RouteAttribute string `mapstructure:"route_attribute"`
	// MethodAttribute is the span attribute key carrying the HTTP method.
	MethodAttribute string `mapstructure:"method_attribute"`
	// SpanKinds is the set of span kinds to measure. Valid values are
	// "unspecified", "internal", "server", "client", "producer" and
	// "consumer". Defaults to all kinds.
	SpanKinds []string `mapstructure:"span_kinds"`
	// MetricName is the name of the emitted latency metric.
	MetricName string `mapstructure:"metric_name"`
}

func createDefaultConfig() component.Config {
	return &Config{
		Percentiles:            defaultPercentiles(),
		Interval:               defaultInterval,
		IntegrationIDAttribute: defaultIntegrationIDAttribute,
		RouteAttribute:         defaultRouteAttribute,
		MethodAttribute:        defaultMethodAttribute,
		SpanKinds:              allSpanKinds(),
		MetricName:             defaultMetricName,
	}
}

// Validate implements component.ConfigValidator.
func (c *Config) Validate() error {
	if c.Interval <= 0 {
		return fmt.Errorf("interval must be positive, got %s", c.Interval)
	}
	if len(c.Percentiles) == 0 {
		return fmt.Errorf("at least one percentile must be configured")
	}
	for _, p := range c.Percentiles {
		if p <= 0 || p >= 1 {
			return fmt.Errorf("percentile must be in the open interval (0, 1), got %v", p)
		}
	}
	if c.IntegrationIDAttribute == "" {
		return fmt.Errorf("integration_id_attribute must not be empty")
	}
	if c.RouteAttribute == "" {
		return fmt.Errorf("route_attribute must not be empty")
	}
	if c.MethodAttribute == "" {
		return fmt.Errorf("method_attribute must not be empty")
	}
	if len(c.SpanKinds) == 0 {
		return fmt.Errorf("at least one span kind must be configured")
	}
	for _, kind := range c.SpanKinds {
		if !isKnownSpanKind(kind) {
			return fmt.Errorf("unknown span kind %q, valid values are %v", kind, allSpanKinds())
		}
	}
	if c.MetricName == "" {
		return fmt.Errorf("metric_name must not be empty")
	}
	return nil
}

func isKnownSpanKind(kind string) bool {
	for _, known := range allSpanKinds() {
		if kind == known {
			return true
		}
	}
	return false
}
