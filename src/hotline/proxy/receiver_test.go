package proxy_test

import (
	"context"
	"sync"
	"time"

	collogs "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	colmetrics "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	coltrace "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	common "go.opentelemetry.io/proto/otlp/common/v1"
	metrics "go.opentelemetry.io/proto/otlp/metrics/v1"
	"google.golang.org/grpc"
)

// receivedRecord is one log record the fake OTLP receiver got.
type receivedRecord struct {
	eventName string
	resource  map[string]any
	attrs     map[string]any
}

// receivedCount is one data point of the hotline.requests counter.
type receivedCount struct {
	unit        string
	monotonic   bool
	temporality metrics.AggregationTemporality
	attrs       map[string]any
	value       int64
}

// receiver is a fake OTLP destination on a local port.
type receiver struct {
	collogs.UnimplementedLogsServiceServer

	server   *grpc.Server
	endpoint string

	mu          sync.Mutex
	records     []receivedRecord
	counts      []receivedCount
	histograms  int
	otherMetric []string
	spanCount   int
	// logsHeld counts log exports that are waiting on hold.
	logsHeld int
	// hold, while open, keeps every export waiting.
	hold chan struct{}
	// delay slows every export down.
	delay time.Duration
}

func newReceiver() *receiver {
	listener := listenLocal()
	r := &receiver{server: grpc.NewServer(), endpoint: listener.Addr().String()}
	collogs.RegisterLogsServiceServer(r.server, r)
	colmetrics.RegisterMetricsServiceServer(r.server, metricsService{receiver: r})
	coltrace.RegisterTraceServiceServer(r.server, traceService{receiver: r})
	go func() { _ = r.server.Serve(listener) }()
	return r
}

func (r *receiver) wait(ctx context.Context) {
	r.mu.Lock()
	hold, delay := r.hold, r.delay
	r.mu.Unlock()
	if hold != nil {
		select {
		case <-hold:
		case <-ctx.Done():
		}
	}
	select {
	case <-time.After(delay):
	case <-ctx.Done():
	}
}

func (r *receiver) Export(ctx context.Context, req *collogs.ExportLogsServiceRequest) (*collogs.ExportLogsServiceResponse, error) {
	r.mu.Lock()
	if r.hold != nil {
		r.logsHeld++
	}
	r.mu.Unlock()
	r.wait(ctx)
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, rl := range req.GetResourceLogs() {
		res := values(rl.GetResource().GetAttributes())
		for _, sl := range rl.GetScopeLogs() {
			for _, lr := range sl.GetLogRecords() {
				r.records = append(r.records, receivedRecord{
					eventName: lr.GetEventName(),
					resource:  res,
					attrs:     values(lr.GetAttributes()),
				})
			}
		}
	}
	return &collogs.ExportLogsServiceResponse{}, nil
}

type metricsService struct {
	colmetrics.UnimplementedMetricsServiceServer
	*receiver
}

func (m metricsService) Export(ctx context.Context, req *colmetrics.ExportMetricsServiceRequest) (*colmetrics.ExportMetricsServiceResponse, error) {
	return m.exportMetrics(ctx, req)
}

func (r *receiver) exportMetrics(ctx context.Context, req *colmetrics.ExportMetricsServiceRequest) (*colmetrics.ExportMetricsServiceResponse, error) {
	r.wait(ctx)
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, rm := range req.GetResourceMetrics() {
		for _, sm := range rm.GetScopeMetrics() {
			for _, m := range sm.GetMetrics() {
				if m.GetHistogram() != nil || m.GetExponentialHistogram() != nil {
					r.histograms++
				}
				if m.GetName() != "hotline.requests" {
					r.otherMetric = append(r.otherMetric, m.GetName())
					continue
				}
				sum := m.GetSum()
				for _, dp := range sum.GetDataPoints() {
					r.counts = append(r.counts, receivedCount{
						unit:        m.GetUnit(),
						monotonic:   sum.GetIsMonotonic(),
						temporality: sum.GetAggregationTemporality(),
						attrs:       values(dp.GetAttributes()),
						value:       dp.GetAsInt(),
					})
				}
			}
		}
	}
	return &colmetrics.ExportMetricsServiceResponse{}, nil
}

type traceService struct {
	coltrace.UnimplementedTraceServiceServer
	*receiver
}

func (t traceService) Export(_ context.Context, req *coltrace.ExportTraceServiceRequest) (*coltrace.ExportTraceServiceResponse, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, rs := range req.GetResourceSpans() {
		for _, ss := range rs.GetScopeSpans() {
			t.spanCount += len(ss.GetSpans())
		}
	}
	return &coltrace.ExportTraceServiceResponse{}, nil
}

func (r *receiver) spans() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.spanCount
}

// waitForLogsHeld waits until a log export is held.
func (r *receiver) waitForLogsHeld() error {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		held := r.logsHeld
		r.mu.Unlock()
		if held > 0 {
			return nil
		}
		time.Sleep(5 * time.Millisecond)
	}
	return failf("no log export arrived")
}

// release lets every held export go on.
func (r *receiver) release() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.hold != nil {
		close(r.hold)
		r.hold = nil
	}
}

func (r *receiver) got() ([]receivedRecord, []receivedCount) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]receivedRecord(nil), r.records...), append([]receivedCount(nil), r.counts...)
}

func (r *receiver) stop() {
	r.release()
	r.server.Stop()
}

func values(kvs []*common.KeyValue) map[string]any {
	m := map[string]any{}
	for _, kv := range kvs {
		m[kv.GetKey()] = value(kv.GetValue())
	}
	return m
}

func value(v *common.AnyValue) any {
	switch x := v.GetValue().(type) {
	case *common.AnyValue_StringValue:
		return x.StringValue
	case *common.AnyValue_IntValue:
		return x.IntValue
	case *common.AnyValue_DoubleValue:
		return x.DoubleValue
	case *common.AnyValue_BoolValue:
		return x.BoolValue
	case *common.AnyValue_ArrayValue:
		var list []any
		for _, item := range x.ArrayValue.GetValues() {
			list = append(list, value(item))
		}
		return list
	}
	return nil
}
