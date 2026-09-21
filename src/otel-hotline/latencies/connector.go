package latencies

import (
	"context"
	"strconv"
	"sync"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/connector"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"

	"hotline/metrics/tdigest"
)

const (
	metricUnit = "s"

	tdigestCapacity   = 100
	tdigestBufferSize = 500

	integrationIDAttribute = "x-integration-id"
	routeAttribute         = "http.route"
	methodAttribute        = "http.request.method"
	kindAttribute          = "kind"
	quantileAttribute      = "quantile"

	kindUnspecified = "unspecified"
	kindInternal    = "internal"
	kindServer      = "server"
	kindClient      = "client"
	kindProducer    = "producer"
	kindConsumer    = "consumer"
)

// allSpanKinds lists every span kind label, in the order used as the default
// configuration (all kinds measured).
func allSpanKinds() []string {
	return []string{
		kindUnspecified,
		kindInternal,
		kindServer,
		kindClient,
		kindProducer,
		kindConsumer,
	}
}

func spanKindLabel(kind ptrace.SpanKind) string {
	switch kind {
	case ptrace.SpanKindInternal:
		return kindInternal
	case ptrace.SpanKindServer:
		return kindServer
	case ptrace.SpanKindClient:
		return kindClient
	case ptrace.SpanKindProducer:
		return kindProducer
	case ptrace.SpanKindConsumer:
		return kindConsumer
	default:
		return kindUnspecified
	}
}

type seriesKey struct {
	integrationID string
	route         string
	method        string
	kind          string
}

type latenciesConnector struct {
	cfg          *Config
	logger       *zap.Logger
	next         consumer.Metrics
	enabledKinds map[string]bool

	mu      sync.Mutex
	digests map[seriesKey]*tdigest.TDSketch

	ticker    *time.Ticker
	doneCh    chan struct{}
	cancelRun context.CancelFunc
	stopOnce  sync.Once
}

func newLatenciesConnector(set connector.Settings, cfg *Config, next consumer.Metrics) *latenciesConnector {
	enabledKinds := make(map[string]bool, len(cfg.SpanKinds))
	for _, kind := range cfg.SpanKinds {
		enabledKinds[kind] = true
	}
	return &latenciesConnector{
		cfg:          cfg,
		logger:       set.Logger,
		next:         next,
		enabledKinds: enabledKinds,
		digests:      make(map[seriesKey]*tdigest.TDSketch),
		doneCh:       make(chan struct{}),
	}
}

func (c *latenciesConnector) Capabilities() consumer.Capabilities {
	return consumer.Capabilities{MutatesData: false}
}

func (c *latenciesConnector) Start(_ context.Context, _ component.Host) error {
	c.ticker = time.NewTicker(c.cfg.Interval)
	// The collector cancels the Start context once startup completes, so the
	// emit loop is given its own context, cancelled on Shutdown instead.
	runCtx, cancel := context.WithCancel(context.Background())
	c.cancelRun = cancel
	go c.run(runCtx)
	c.logger.Info(
		"latencies connector started",
		zap.String("interval", c.cfg.Interval.String()),
	)
	return nil
}

func (c *latenciesConnector) Shutdown(ctx context.Context) error {
	c.stopOnce.Do(func() {
		if c.ticker != nil {
			c.ticker.Stop()
		}
		close(c.doneCh)
	})
	// Emit whatever has accumulated since the last tick.
	err := c.flush(ctx, time.Now())
	// Released only once the final flush is done, so shutdown never cancels a
	// batch that is already being delivered.
	if c.cancelRun != nil {
		c.cancelRun()
	}
	return err
}

func (c *latenciesConnector) run(ctx context.Context) {
	for {
		select {
		case <-c.doneCh:
			return
		case now := <-c.ticker.C:
			if err := c.flush(ctx, now); err != nil {
				c.logger.Error("failed to emit latency metrics", zap.Error(err))
			}
		}
	}
}

func (c *latenciesConnector) ConsumeTraces(_ context.Context, td ptrace.Traces) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	resourceSpans := td.ResourceSpans()
	for i := range resourceSpans.Len() {
		scopeSpans := resourceSpans.At(i).ScopeSpans()
		for j := range scopeSpans.Len() {
			spans := scopeSpans.At(j).Spans()
			for k := range spans.Len() {
				c.recordSpan(spans.At(k))
			}
		}
	}
	return nil
}

func (c *latenciesConnector) recordSpan(span ptrace.Span) {
	kind := spanKindLabel(span.Kind())
	if !c.enabledKinds[kind] {
		return
	}

	attrs := span.Attributes()
	integrationID, ok := stringAttr(attrs, c.cfg.IntegrationIDAttribute)
	if !ok {
		return
	}
	route, ok := stringAttr(attrs, c.cfg.RouteAttribute)
	if !ok {
		return
	}
	method, ok := stringAttr(attrs, c.cfg.MethodAttribute)
	if !ok {
		return
	}

	latencySeconds := durationSeconds(span.StartTimestamp(), span.EndTimestamp())
	if latencySeconds < 0 {
		return
	}

	key := seriesKey{integrationID: integrationID, route: route, method: method, kind: kind}
	digest, found := c.digests[key]
	if !found {
		digest = tdigest.NewWeightScaledTDSketch(tdigestCapacity, tdigestBufferSize)
		c.digests[key] = digest
	}
	digest.AddToBuffer(latencySeconds, 1)
}

// flush computes the configured percentiles for every active series, emits
// them as a single metrics batch and resets the accumulators (tumbling
// window with delta semantics).
func (c *latenciesConnector) flush(ctx context.Context, now time.Time) error {
	c.mu.Lock()
	digests := c.digests
	c.digests = make(map[seriesKey]*tdigest.TDSketch)
	c.mu.Unlock()

	if len(digests) == 0 {
		return nil
	}

	md := c.buildMetrics(digests, now)
	return c.next.ConsumeMetrics(ctx, md)
}

func (c *latenciesConnector) buildMetrics(digests map[seriesKey]*tdigest.TDSketch, now time.Time) pmetric.Metrics {
	md := pmetric.NewMetrics()
	sm := md.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty()
	metric := sm.Metrics().AppendEmpty()
	metric.SetName(c.cfg.MetricName)
	metric.SetUnit(metricUnit)
	dps := metric.SetEmptyGauge().DataPoints()

	ts := pcommon.NewTimestampFromTime(now)
	for key, digest := range digests {
		for _, percentile := range c.cfg.Percentiles {
			dp := dps.AppendEmpty()
			dp.SetTimestamp(ts)
			dp.SetDoubleValue(digest.Quantile(percentile))
			dp.Attributes().PutStr(integrationIDAttribute, key.integrationID)
			dp.Attributes().PutStr(routeAttribute, key.route)
			dp.Attributes().PutStr(methodAttribute, key.method)
			dp.Attributes().PutStr(kindAttribute, key.kind)
			dp.Attributes().PutStr(quantileAttribute, strconv.FormatFloat(percentile, 'g', -1, 64))
		}
	}
	return md
}

func stringAttr(attrs pcommon.Map, key string) (string, bool) {
	v, ok := attrs.Get(key)
	if !ok {
		return "", false
	}
	return v.AsString(), true
}

func durationSeconds(start, end pcommon.Timestamp) float64 {
	return float64(end.AsTime().Sub(start.AsTime())) / float64(time.Second)
}
