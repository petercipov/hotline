package latencies

import (
	"errors"
	"fmt"
	"time"

	"go.opentelemetry.io/collector/component"

	"hotline/metrics/ddsketch"
)

// Type is the component type of this connector. component.MustNewType is a
// function call, so this cannot be a constant.
//
//nolint:gochecknoglobals // the collector component API requires a package level type
var Type = component.MustNewType("latencies")

// Validation failures are static errors so a caller can match on them with
// errors.Is rather than on the message text.
var (
	ErrNonPositiveInterval = errors.New("latencies: interval must be positive")
	ErrNoPercentiles       = errors.New("latencies: at least one percentile must be configured")
	ErrPercentileRange     = errors.New("latencies: percentile must be in the open interval (0, 1)")
	ErrEmptyAttribute      = errors.New("latencies: attribute name must not be empty")
	ErrNoSpanKinds         = errors.New("latencies: at least one span kind must be configured")
	ErrUnknownSpanKind     = errors.New("latencies: unknown span kind")
	ErrEmptyMetricName     = errors.New("latencies: metric_name must not be empty")
)

const (
	defaultIntegrationIDAttribute = "x-integration-id"
	defaultRouteAttribute         = "http.route"
	defaultMethodAttribute        = "http.request.method"
	defaultInterval               = 10 * time.Second
	defaultWindow                 = ddsketch.DefaultWindow
	defaultMaxLateness            = time.Minute
	defaultMetricName             = "http.span.request.duration"
)

func defaultPercentiles() []float64 {
	return []float64{0.99, 0.8, 0.75}
}

// Config configures the latencies connector that turns HTTP server span
// durations into latency percentile metrics per integration id and route.
//
// A full configuration, every key at its default:
//
//	connectors:
//	  latencies:
//	    percentiles: [0.99, 0.8, 0.75]
//	    interval: 10s
//	    window: 1h
//	    max_lateness: 1m
//	    integration_id_attribute: x-integration-id
//	    route_attribute: http.route
//	    method_attribute: http.request.method
//	    span_kinds: [unspecified, internal, server, client, producer, consumer]
//	    metric_name: http.span.request.duration
type Config struct {
	// Percentiles is the set of quantiles to compute, each in the open
	// interval (0, 1). Defaults to p99, p80, p75.
	//
	//	percentiles: [0.5, 0.95, 0.99]   # p50, p95 and p99
	Percentiles []float64 `mapstructure:"percentiles"`
	// Interval is how often percentile metrics are emitted. It is the emit
	// cadence only: every tick reports the whole Window, which slides with
	// the clock rather than tumbling between ticks.
	//
	//	interval: 10s   # publish the window every ten seconds
	Interval time.Duration `mapstructure:"interval"`
	// Window is the span of latencies each emitted percentile covers, ending
	// at the tick that reports it. Whole seconds only. Defaults to one hour.
	//
	//	window: 1h    # each p99 describes the last hour
	//	window: 5m    # shorter window, reacts faster, less memory
	Window time.Duration `mapstructure:"window"`
	// MaxLateness is how long after a second has passed a span for it may
	// still arrive and be counted in that second rather than dropped. Whole
	// seconds, zero allowed. It costs retention beyond the window, so memory
	// is proportional to Window plus MaxLateness.
	//
	//	max_lateness: 1m   # a span up to a minute late still counts
	//	max_lateness: 0s   # count a span only while its second is in the window
	MaxLateness time.Duration `mapstructure:"max_lateness"`
	// IntegrationIDAttribute is the span attribute key carrying the
	// integration id used to partition metrics.
	//
	//	integration_id_attribute: x-integration-id   # one series per integration
	IntegrationIDAttribute string `mapstructure:"integration_id_attribute"`
	// RouteAttribute is the span attribute key carrying the HTTP route.
	//
	//	route_attribute: http.route   # the templated path, e.g. /v1/orders/{id}
	RouteAttribute string `mapstructure:"route_attribute"`
	// MethodAttribute is the span attribute key carrying the HTTP method.
	//
	//	method_attribute: http.request.method   # GET, POST, ...
	MethodAttribute string `mapstructure:"method_attribute"`
	// SpanKinds is the set of span kinds to measure. Valid values are
	// "unspecified", "internal", "server", "client", "producer" and
	// "consumer". Defaults to all kinds.
	//
	//	span_kinds: [client]           # only calls we make to third parties
	//	span_kinds: [server, client]   # both directions, as separate series
	SpanKinds []string `mapstructure:"span_kinds"`
	// MetricName is the name of the emitted latency metric.
	//
	//	metric_name: http.span.request.duration   # gauge, in seconds
	MetricName string `mapstructure:"metric_name"`
}

func createDefaultConfig() component.Config {
	return &Config{
		Percentiles:            defaultPercentiles(),
		Interval:               defaultInterval,
		Window:                 defaultWindow,
		MaxLateness:            defaultMaxLateness,
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
		return fmt.Errorf("%w, got %s", ErrNonPositiveInterval, c.Interval)
	}
	// the sketch owns what a window and a lateness may be, so they are
	// checked by building one here rather than restated and left to drift
	if _, err := ddsketch.NewSlidingWindowSketch(c.Window, c.MaxLateness); err != nil {
		return fmt.Errorf("latencies: %w", err)
	}
	if len(c.Percentiles) == 0 {
		return ErrNoPercentiles
	}
	for _, p := range c.Percentiles {
		if p <= 0 || p >= 1 {
			return fmt.Errorf("%w, got %v", ErrPercentileRange, p)
		}
	}
	if c.IntegrationIDAttribute == "" {
		return fmt.Errorf("%w: integration_id_attribute", ErrEmptyAttribute)
	}
	if c.RouteAttribute == "" {
		return fmt.Errorf("%w: route_attribute", ErrEmptyAttribute)
	}
	if c.MethodAttribute == "" {
		return fmt.Errorf("%w: method_attribute", ErrEmptyAttribute)
	}
	if len(c.SpanKinds) == 0 {
		return ErrNoSpanKinds
	}
	for _, kind := range c.SpanKinds {
		if !isKnownSpanKind(kind) {
			return fmt.Errorf("%w %q, valid values are %v", ErrUnknownSpanKind, kind, allSpanKinds())
		}
	}
	if c.MetricName == "" {
		return ErrEmptyMetricName
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
