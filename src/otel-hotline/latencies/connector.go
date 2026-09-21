package latencies

import (
	"context"
	"math"
	"slices"
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

	"hotline/metrics/ddsketch"
)

const (
	metricUnit = "s"

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

// seriesValue is one series' sliding window, with the most recent second it
// has seen, on which reclaiming an idle series depends.
type seriesValue struct {
	windowSketch *ddsketch.SlidingWindowSketch
	lastSec      uint64
}

type latenciesConnector struct {
	cfg          *Config
	logger       *zap.Logger
	next         consumer.Metrics
	enabledKinds map[string]bool
	// percentiles is the configured set sorted ascending, as reading every
	// quantile in one pass requires. Emitted attributes follow this order
	// rather than the configured one.
	percentiles []float64
	// retentionSec is Window plus MaxLateness in seconds, which is how far
	// back a series stays writable.
	retentionSec uint64

	mu     sync.Mutex
	series map[seriesKey]*seriesValue
	// horizonSec is the oldest second still accepted, tracked here so a span
	// far behind the clock does not open a window of its own.
	horizonSec uint64

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
	percentiles := slices.Clone(cfg.Percentiles)
	slices.Sort(percentiles)
	return &latenciesConnector{
		cfg:          cfg,
		logger:       set.Logger,
		next:         next,
		enabledKinds: enabledKinds,
		percentiles:  percentiles,
		retentionSec: retentionSeconds(cfg.Window, cfg.MaxLateness),
		series:       make(map[seriesKey]*seriesValue),
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

	latencyNS := durationNanos(span.StartTimestamp(), span.EndTimestamp())
	if latencyNS < 0 {
		return
	}

	// the window is keyed by when the span happened, not by when it reached
	// the collector, so a late batch lands on the second it belongs to
	timestampNS := uint64(span.EndTimestamp())
	tsSec := timestampNS / ddsketch.NanosPerSecond
	if tsSec < c.horizonSec {
		c.logger.Debug(
			"dropped latency measurement below the retention horizon",
			zap.Uint64("second", tsSec),
			zap.Uint64("horizon", c.horizonSec),
		)
		return
	}

	key := seriesKey{integrationID: integrationID, route: route, method: method, kind: kind}
	seriesContent, found := c.series[key]
	if !found {
		// the config is validated by building one of these, so this cannot
		// fail for a config the collector accepted
		windowSketch, err := ddsketch.NewSlidingWindowSketch(c.cfg.Window, c.cfg.MaxLateness)
		if err != nil {
			c.logger.Error("failed to open latency window", zap.Error(err))
			return
		}
		seriesContent = &seriesValue{windowSketch: windowSketch}
		c.series[key] = seriesContent
	}

	if err := seriesContent.windowSketch.Add(timestampNS, latencyNS); err != nil {
		c.logger.Debug("dropped latency measurement", zap.Error(err))
		return
	}
	seriesContent.lastSec = max(seriesContent.lastSec, tsSec)
}

// flush computes the configured percentiles for every active series and emits
// them as a single metrics batch. The windows slide rather than tumble: a
// series stays reported for as long as its window holds measurements, so the
// emitted gauge always describes the configured window ending at this tick.
func (c *latenciesConnector) flush(ctx context.Context, now time.Time) error {
	md, ok := c.collect(now)
	if !ok {
		return nil
	}
	return c.next.ConsumeMetrics(ctx, md)
}

// collect folds every series' window at now, moves the retention horizon
// forward and reclaims the series whose windows have drained. The lock covers
// the whole fold, because quantiles are read from the same sketches
// ConsumeTraces writes into, and one is not safe for concurrent use.
func (c *latenciesConnector) collect(now time.Time) (pmetric.Metrics, bool) {
	nowSec := unixSeconds(now)

	c.mu.Lock()
	defer c.mu.Unlock()

	md := pmetric.NewMetrics()
	sm := md.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty()
	metric := sm.Metrics().AppendEmpty()
	metric.SetName(c.cfg.MetricName)
	metric.SetUnit(metricUnit)
	dps := metric.SetEmptyGauge().DataPoints()

	ts := pcommon.NewTimestampFromTime(now)
	emitted := 0
	for key, seriesContent := range c.series {
		quantiles := seriesContent.windowSketch.Quantiles(nowSec, c.percentiles)
		for i, percentile := range c.percentiles {
			// an empty window answers NaN, which is a series that has gone
			// quiet rather than a latency worth reporting
			if math.IsNaN(quantiles[i]) {
				continue
			}
			dp := dps.AppendEmpty()
			dp.SetTimestamp(ts)
			dp.SetDoubleValue(quantiles[i] / float64(time.Second))
			dp.Attributes().PutStr(integrationIDAttribute, key.integrationID)
			dp.Attributes().PutStr(routeAttribute, key.route)
			dp.Attributes().PutStr(methodAttribute, key.method)
			dp.Attributes().PutStr(kindAttribute, key.kind)
			dp.Attributes().PutStr(quantileAttribute, strconv.FormatFloat(percentile, 'g', -1, 64))
			emitted++
		}

		seriesContent.windowSketch.Advance(nowSec)
		if nowSec > seriesContent.lastSec+c.retentionSec {
			delete(c.series, key)
		}
	}

	if nowSec > c.retentionSec {
		c.horizonSec = nowSec - c.retentionSec
	}

	return md, emitted > 0
}

func stringAttr(attrs pcommon.Map, key string) (string, bool) {
	v, ok := attrs.Get(key)
	if !ok {
		return "", false
	}
	return v.AsString(), true
}

func durationNanos(start, end pcommon.Timestamp) int64 {
	return int64(end.AsTime().Sub(start.AsTime()))
}

// retentionSeconds counts how far back a series stays writable. A config the
// collector accepted cannot be negative here, and one that somehow is retains
// nothing rather than wrapping into an unreachable horizon.
func retentionSeconds(window, maxLateness time.Duration) uint64 {
	retained := window + maxLateness
	if retained < 0 {
		return 0
	}
	return uint64(retained / time.Second)
}

// unixSeconds is now as a window key. A clock before the epoch has no window
// behind it, so it reads as second zero rather than wrapping.
func unixSeconds(t time.Time) uint64 {
	sec := t.Unix()
	if sec < 0 {
		return 0
	}
	return uint64(sec)
}
