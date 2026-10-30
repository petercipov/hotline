// Package export sends records and the hotline.requests counter over OTLP.
// Every destination gets its own queue, so one that is down never holds up
// the others or the calls.
package export

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"log/slog"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/metric"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"google.golang.org/grpc/credentials"

	"hotline-proxy/internal/config"
)

// Timing says how often the proxy sends, and how long one send to a
// destination may take, retries included.
type Timing struct {
	Interval time.Duration
	Timeout  time.Duration
}

// Service describes the proxy on every signal it sends.
type Service struct {
	Version    string
	InstanceID string
}

// Sink takes finished records and sends them to every destination.
type Sink struct {
	logs     *sdklog.LoggerProvider
	metrics  *sdkmetric.MeterProvider
	logger   log.Logger
	requests metric.Int64Counter
}

// counted are the record attributes the hotline.requests counter carries.
func counted(kv attribute.KeyValue) bool {
	switch kv.Key {
	case "hotline.integration.id", semconv.URLTemplateKey, semconv.HTTPRequestMethodKey,
		semconv.HTTPResponseStatusCodeKey, semconv.ErrorTypeKey:
		return true
	}
	return false
}

// New starts one queue and sender per destination. Nothing is sent to a
// destination before the first record, so a destination that is down does
// not stop the proxy from starting. TLS to a destination trusts roots and
// the destination's own CAs.
//
// Only the config says how to send: every option is set here, so the
// library never falls back to the OTEL_EXPORTER_OTLP_* environment
// variables.
func New(exporters []config.Exporter, roots *x509.CertPool, service Service, timing Timing, logger *slog.Logger) *Sink {
	res := resource.NewSchemaless(
		semconv.ServiceName("hotline-proxy"),
		semconv.ServiceVersion(service.Version),
		semconv.ServiceInstanceID(service.InstanceID),
	)
	logOptions := []sdklog.LoggerProviderOption{sdklog.WithResource(res)}
	metricOptions := []sdkmetric.Option{sdkmetric.WithResource(res)}
	for _, e := range exporters {
		var tlsConfig *tls.Config
		if e.URL.Scheme == "https" {
			pool := roots.Clone()
			pool.AppendCertsFromPEM(e.CAs)
			tlsConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
			if e.Certificate != nil {
				tlsConfig.Certificates = []tls.Certificate{*e.Certificate}
			}
		}
		temporality := sdkmetric.DefaultTemporalitySelector
		if e.Delta {
			temporality = delta
		}
		senders := grpcSenders
		if e.HTTP {
			senders = httpSenders
		}
		logs, metrics := senders(e, tlsConfig, timing, temporality)
		logOptions = append(logOptions, sdklog.WithProcessor(newQueue(logs, e.Endpoint, e.QueueSize, timing.Interval, logger)))
		metricOptions = append(metricOptions, sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metrics,
			sdkmetric.WithInterval(timing.Interval),
			sdkmetric.WithTimeout(timing.Timeout))))
	}
	s := &Sink{
		logs:    sdklog.NewLoggerProvider(logOptions...),
		metrics: sdkmetric.NewMeterProvider(metricOptions...),
	}
	s.logger = s.logs.Logger("hotline-proxy")
	// The name and unit are fixed and valid, so there is no error to handle.
	s.requests, _ = s.metrics.Meter("hotline-proxy").Int64Counter("hotline.requests", metric.WithUnit("{request}"))
	return s
}

// A busy destination is tried again, first after about a second, then at
// most every few seconds, until the send's time is up.
const (
	retryFirst = time.Second
	retryMost  = 5 * time.Second
)

// grpcSenders send OTLP over gRPC. The config has checked the endpoint,
// the only thing New could refuse, so there is no error to handle.
func grpcSenders(e config.Exporter, tlsConfig *tls.Config, timing Timing, temporality sdkmetric.TemporalitySelector) (sdklog.Exporter, sdkmetric.Exporter) {
	compressor := "none"
	if e.Gzip {
		compressor = "gzip"
	}
	logOptions := []otlploggrpc.Option{
		otlploggrpc.WithEndpoint(e.URL.Host),
		otlploggrpc.WithHeaders(e.Headers),
		otlploggrpc.WithCompressor(compressor),
		otlploggrpc.WithTimeout(timing.Timeout),
		otlploggrpc.WithRetry(otlploggrpc.RetryConfig{Enabled: true, InitialInterval: retryFirst, MaxInterval: retryMost, MaxElapsedTime: timing.Timeout}),
		otlploggrpc.WithInsecure(),
	}
	metricOptions := []otlpmetricgrpc.Option{
		otlpmetricgrpc.WithEndpoint(e.URL.Host),
		otlpmetricgrpc.WithHeaders(e.Headers),
		otlpmetricgrpc.WithCompressor(compressor),
		otlpmetricgrpc.WithTimeout(timing.Timeout),
		otlpmetricgrpc.WithRetry(otlpmetricgrpc.RetryConfig{Enabled: true, InitialInterval: retryFirst, MaxInterval: retryMost, MaxElapsedTime: timing.Timeout}),
		otlpmetricgrpc.WithTemporalitySelector(temporality),
		otlpmetricgrpc.WithInsecure(),
	}
	if tlsConfig != nil {
		creds := credentials.NewTLS(tlsConfig)
		logOptions[len(logOptions)-1] = otlploggrpc.WithTLSCredentials(creds)
		metricOptions[len(metricOptions)-1] = otlpmetricgrpc.WithTLSCredentials(creds)
	}
	logs, _ := otlploggrpc.New(context.Background(), logOptions...)
	metrics, _ := otlpmetricgrpc.New(context.Background(), metricOptions...)
	return logs, metrics
}

// httpSenders send OTLP over HTTP as protobuf, to /v1/logs and /v1/metrics
// after the endpoint's own path. The config has checked the endpoint, so
// there is no error to handle.
func httpSenders(e config.Exporter, tlsConfig *tls.Config, timing Timing, temporality sdkmetric.TemporalitySelector) (sdklog.Exporter, sdkmetric.Exporter) {
	base := strings.TrimSuffix(e.URL.Path, "/")
	logCompression, metricCompression := otlploghttp.NoCompression, otlpmetrichttp.NoCompression
	if e.Gzip {
		logCompression, metricCompression = otlploghttp.GzipCompression, otlpmetrichttp.GzipCompression
	}
	// A plain endpoint still sets a TLS config, nil, so none is read from
	// the environment.
	logOptions := []otlploghttp.Option{
		otlploghttp.WithEndpoint(e.URL.Host),
		otlploghttp.WithURLPath(base + "/v1/logs"),
		otlploghttp.WithHeaders(e.Headers),
		otlploghttp.WithCompression(logCompression),
		otlploghttp.WithTimeout(timing.Timeout),
		otlploghttp.WithRetry(otlploghttp.RetryConfig{Enabled: true, InitialInterval: retryFirst, MaxInterval: retryMost, MaxElapsedTime: timing.Timeout}),
		otlploghttp.WithTLSClientConfig(tlsConfig),
	}
	metricOptions := []otlpmetrichttp.Option{
		otlpmetrichttp.WithEndpoint(e.URL.Host),
		otlpmetrichttp.WithURLPath(base + "/v1/metrics"),
		otlpmetrichttp.WithHeaders(e.Headers),
		otlpmetrichttp.WithCompression(metricCompression),
		otlpmetrichttp.WithTimeout(timing.Timeout),
		otlpmetrichttp.WithRetry(otlpmetrichttp.RetryConfig{Enabled: true, InitialInterval: retryFirst, MaxInterval: retryMost, MaxElapsedTime: timing.Timeout}),
		otlpmetrichttp.WithTemporalitySelector(temporality),
		otlpmetrichttp.WithTLSClientConfig(tlsConfig),
	}
	if tlsConfig == nil {
		logOptions = append(logOptions, otlploghttp.WithInsecure())
		metricOptions = append(metricOptions, otlpmetrichttp.WithInsecure())
	}
	logs, _ := otlploghttp.New(context.Background(), logOptions...)
	metrics, _ := otlpmetrichttp.New(context.Background(), metricOptions...)
	return logs, metrics
}

func delta(sdkmetric.InstrumentKind) metricdata.Temporality {
	return metricdata.DeltaTemporality
}

// Record queues one hotline.request log record and counts the call. It never
// blocks: a full queue drops its oldest record.
func (s *Sink) Record(ctx context.Context, at time.Time, attrs []attribute.KeyValue) {
	var r log.Record
	r.SetEventName("hotline.request")
	r.SetTimestamp(at)
	var counter []attribute.KeyValue
	for _, kv := range attrs {
		r.AddAttributes(kv)
		if counted(kv) {
			counter = append(counter, kv)
		}
	}
	s.logger.Emit(ctx, r)
	s.requests.Add(ctx, 1, metric.WithAttributes(counter...))
}

// Shutdown sends what is still queued, within the time ctx allows.
func (s *Sink) Shutdown(ctx context.Context) error {
	return errors.Join(s.logs.Shutdown(ctx), s.metrics.Shutdown(ctx))
}
