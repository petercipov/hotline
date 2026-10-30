package proxy_test

import (
	"compress/gzip"
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	collogs "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	colmetrics "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	coltrace "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	common "go.opentelemetry.io/proto/otlp/common/v1"
	metrics "go.opentelemetry.io/proto/otlp/metrics/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	// gzip lets the gRPC side take gzipped exports.
	_ "google.golang.org/grpc/encoding/gzip"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/stats"
	"google.golang.org/protobuf/proto"
)

// receivedRecord is one log record the fake OTLP receiver got.
type receivedRecord struct {
	eventName string
	// severityText and severityNumber are the record's level.
	severityText   string
	severityNumber int32
	resource       map[string]any
	attrs          map[string]any
}

// receivedCount is one data point of the hotline.requests counter.
type receivedCount struct {
	unit        string
	monotonic   bool
	temporality metrics.AggregationTemporality
	attrs       map[string]any
	value       int64
}

// seenRequest is one export request the receiver got, of either kind.
type seenRequest struct {
	logs        bool
	path        string
	contentType string
	// encoding is gzip or empty.
	encoding      string
	authorization []string
	// header holds every header of an HTTP request.
	header     http.Header
	tls        bool
	clientCert bool
}

// answer is what the HTTP side answers a log export with, instead of
// taking it.
type answer struct {
	status     int
	retryAfter string
}

// receiverSetup says how a fake receiver listens.
type receiverSetup struct {
	http bool
	// path goes in front of /v1/logs and /v1/metrics.
	path string
	// tls, when set, makes the receiver accept only TLS, with this setup.
	tls *tls.Config
}

// receiver is a fake OTLP destination on a local port. It has a gRPC side
// or an HTTP side.
type receiver struct {
	collogs.UnimplementedLogsServiceServer

	server     *grpc.Server
	httpServer *http.Server
	http       bool
	// endpoint is how the config names the receiver.
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
	// seen are the export requests, in the order they came.
	seen []seenRequest
	// answers are given to the first log exports over HTTP, one each.
	answers []answer
	// encodings are the compression of each gRPC export, as the gRPC
	// side saw it.
	encodings []string
}

func newReceiver() *receiver {
	return newReceiverWith(receiverSetup{})
}

func newReceiverWith(setup receiverSetup) *receiver {
	listener := listenLocal()
	_, port, _ := net.SplitHostPort(listener.Addr().String())
	r := &receiver{http: setup.http, endpoint: listener.Addr().String()}
	if setup.tls != nil {
		r.endpoint = "https://localhost:" + port
	}
	if setup.http {
		if setup.tls == nil {
			r.endpoint = "http://" + r.endpoint
		}
		r.endpoint += setup.path
		r.httpServer = &http.Server{Handler: r, TLSConfig: setup.tls, ReadHeaderTimeout: 5 * time.Second}
		if setup.tls != nil {
			listener = tls.NewListener(listener, setup.tls)
		}
		go func() { _ = r.httpServer.Serve(listener) }()
		return r
	}
	options := []grpc.ServerOption{grpc.StatsHandler(encodingHandler{r})}
	if setup.tls != nil {
		options = append(options, grpc.Creds(credentials.NewTLS(setup.tls)))
	}
	r.server = grpc.NewServer(options...)
	collogs.RegisterLogsServiceServer(r.server, r)
	colmetrics.RegisterMetricsServiceServer(r.server, metricsService{receiver: r})
	coltrace.RegisterTraceServiceServer(r.server, traceService{receiver: r})
	go func() { _ = r.server.Serve(listener) }()
	return r
}

// ServeHTTP is the HTTP side: protobuf exports on /v1/logs and /v1/metrics,
// after any path the endpoint has.
func (r *receiver) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	logs := strings.HasSuffix(req.URL.Path, "/v1/logs")
	r.mu.Lock()
	r.seen = append(r.seen, seenRequest{
		logs:          logs,
		path:          req.URL.Path,
		contentType:   req.Header.Get("Content-Type"),
		encoding:      req.Header.Get("Content-Encoding"),
		authorization: req.Header.Values("Authorization"),
		header:        req.Header.Clone(),
		tls:           req.TLS != nil,
		clientCert:    req.TLS != nil && len(req.TLS.PeerCertificates) > 0,
	})
	var refuse *answer
	if logs && len(r.answers) > 0 {
		refuse = &r.answers[0]
		r.answers = r.answers[1:]
	}
	r.mu.Unlock()
	if refuse != nil {
		if refuse.retryAfter != "" {
			rw.Header().Set("Retry-After", refuse.retryAfter)
		}
		rw.WriteHeader(refuse.status)
		return
	}
	body := io.Reader(req.Body)
	if req.Header.Get("Content-Encoding") == "gzip" {
		unzipped, err := gzip.NewReader(req.Body)
		if err != nil {
			rw.WriteHeader(http.StatusBadRequest)
			return
		}
		body = unzipped
	}
	data, _ := io.ReadAll(body)
	var reply proto.Message
	switch {
	case logs:
		in := &collogs.ExportLogsServiceRequest{}
		if proto.Unmarshal(data, in) != nil {
			rw.WriteHeader(http.StatusBadRequest)
			return
		}
		reply, _ = r.Export(req.Context(), in)
	case strings.HasSuffix(req.URL.Path, "/v1/metrics"):
		in := &colmetrics.ExportMetricsServiceRequest{}
		if proto.Unmarshal(data, in) != nil {
			rw.WriteHeader(http.StatusBadRequest)
			return
		}
		reply, _ = r.exportMetrics(req.Context(), in)
	default:
		rw.WriteHeader(http.StatusNotFound)
		return
	}
	out, _ := proto.Marshal(reply)
	rw.Header().Set("Content-Type", "application/x-protobuf")
	_, _ = rw.Write(out)
}

// sawGRPC remembers a gRPC export request.
func (r *receiver) sawGRPC(ctx context.Context, logs bool) {
	md, _ := metadata.FromIncomingContext(ctx)
	seen := seenRequest{logs: logs, authorization: md.Get("authorization")}
	if p, ok := peer.FromContext(ctx); ok {
		if info, ok := p.AuthInfo.(credentials.TLSInfo); ok {
			seen.tls = true
			seen.clientCert = len(info.State.PeerCertificates) > 0
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seen = append(r.seen, seen)
}

func (r *receiver) requests() []seenRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	seen := append([]seenRequest(nil), r.seen...)
	for i := range seen {
		if !r.http && i < len(r.encodings) {
			seen[i].encoding = r.encodings[i]
		}
	}
	return seen
}

// encodingHandler sees the compression of each gRPC export.
type encodingHandler struct{ *receiver }

func (encodingHandler) TagRPC(ctx context.Context, _ *stats.RPCTagInfo) context.Context { return ctx }

func (h encodingHandler) HandleRPC(_ context.Context, s stats.RPCStats) {
	if in, ok := s.(*stats.InHeader); ok {
		h.mu.Lock()
		h.encodings = append(h.encodings, in.Compression)
		h.mu.Unlock()
	}
}

func (encodingHandler) TagConn(ctx context.Context, _ *stats.ConnTagInfo) context.Context { return ctx }

func (encodingHandler) HandleConn(context.Context, stats.ConnStats) {}

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
	if !r.http {
		r.sawGRPC(ctx, true)
	}
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
					eventName:      lr.GetEventName(),
					severityText:   lr.GetSeverityText(),
					severityNumber: int32(lr.GetSeverityNumber()),
					resource:       res,
					attrs:          values(lr.GetAttributes()),
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
	m.sawGRPC(ctx, false)
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
	if r.server != nil {
		r.server.Stop()
	}
	if r.httpServer != nil {
		_ = r.httpServer.Close()
	}
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
	case *common.AnyValue_KvlistValue:
		return values(x.KvlistValue.GetValues())
	case *common.AnyValue_ArrayValue:
		var list []any
		for _, item := range x.ArrayValue.GetValues() {
			list = append(list, value(item))
		}
		return list
	}
	return nil
}
