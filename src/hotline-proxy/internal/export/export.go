// Package export sends records and the hotline.requests counter over OTLP.
// Every destination gets its own queue, so one that is down never holds up
// the others or the calls.
package export

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/metric"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"

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
// not stop the proxy from starting.
func New(ctx context.Context, exporters []config.Exporter, service Service, timing Timing, logger *slog.Logger) (*Sink, error) {
	res := resource.NewSchemaless(
		semconv.ServiceName("hotline-proxy"),
		semconv.ServiceVersion(service.Version),
		semconv.ServiceInstanceID(service.InstanceID),
	)
	type sender struct {
		config.Exporter
		logs    *otlploggrpc.Exporter
		metrics *otlpmetricgrpc.Exporter
	}
	var senders []sender
	var problems []error
	for _, e := range exporters {
		logs, err := otlploggrpc.New(ctx,
			otlploggrpc.WithEndpoint(e.Endpoint),
			otlploggrpc.WithInsecure(),
			otlploggrpc.WithTimeout(timing.Timeout))
		if err != nil {
			problems = append(problems, wrap(e, err))
			continue
		}
		temporality := sdkmetric.DefaultTemporalitySelector
		if e.Delta {
			temporality = delta
		}
		// The same endpoint was just accepted for logs, so metrics accept it
		// too.
		metrics, _ := otlpmetricgrpc.New(ctx,
			otlpmetricgrpc.WithEndpoint(e.Endpoint),
			otlpmetricgrpc.WithInsecure(),
			otlpmetricgrpc.WithTimeout(timing.Timeout),
			otlpmetricgrpc.WithTemporalitySelector(temporality))
		senders = append(senders, sender{Exporter: e, logs: logs, metrics: metrics})
	}
	if len(problems) > 0 {
		for _, s := range senders {
			problems = append(problems, s.logs.Shutdown(ctx), s.metrics.Shutdown(ctx))
		}
		return nil, errors.Join(problems...)
	}
	logOptions := []sdklog.LoggerProviderOption{sdklog.WithResource(res)}
	metricOptions := []sdkmetric.Option{sdkmetric.WithResource(res)}
	for _, s := range senders {
		logOptions = append(logOptions, sdklog.WithProcessor(newQueue(s.logs, s.Endpoint, s.QueueSize, timing.Interval, logger)))
		metricOptions = append(metricOptions, sdkmetric.WithReader(sdkmetric.NewPeriodicReader(s.metrics,
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
	return s, nil
}

// ErrExporter marks a destination the proxy cannot send to.
var ErrExporter = errors.New("export: unusable exporter")

func wrap(e config.Exporter, err error) error {
	return fmt.Errorf("%w: endpoint %s: %w", ErrExporter, e.Endpoint, err)
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
