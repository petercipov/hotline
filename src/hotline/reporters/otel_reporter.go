package reporters

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	"google.golang.org/protobuf/proto"
	http2 "hotline/http"
	"hotline/servicelevels"
	"net/http"
	"net/url"
	"time"
)

type OtelUrl string

func NewOtelUrl(secure bool, host string) (OtelUrl, error) {
	scheme := "https"
	if !secure {
		scheme = "http"
	}
	otelUrl, parseErr := url.ParseRequestURI(fmt.Sprintf("%s://%s/v1/metrics", scheme, host))
	if parseErr != nil {
		return "", parseErr
	}

	return OtelUrl(otelUrl.String()), nil
}

func (o *OtelUrl) String() string {
	return string(*o)
}

type OtelReporterConfig struct {
	OtelUrl   OtelUrl
	Method    string
	UserAgent string
}

type OtelReporter struct {
	client       *http.Client
	cfg          *OtelReporterConfig
	protoMarshal func(proto.Message) ([]byte, error)
	gzipWriter   *gzip.Writer
}

func NewOtelReporter(cfg *OtelReporterConfig, client *http.Client, gzipWriter *gzip.Writer, protoMarshal func(proto.Message) ([]byte, error)) *OtelReporter {
	return &OtelReporter{
		client:       client,
		cfg:          cfg,
		protoMarshal: protoMarshal,
		gzipWriter:   gzipWriter,
	}
}

func (o *OtelReporter) ReportChecks(ctx context.Context, report *servicelevels.CheckReport) error {
	var allMetrics []*metricspb.Metric
	for _, check := range report.Checks {
		metrics := toMetrics(report.Now, check)
		allMetrics = append(allMetrics, metrics...)
	}

	if len(allMetrics) == 0 {
		return nil
	}

	message := &colmetricspb.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricspb.ResourceMetrics{
			{
				ScopeMetrics: []*metricspb.ScopeMetrics{
					{
						Metrics: allMetrics,
					},
				},
			},
		},
	}

	marshalledBytes, marshalErr := o.protoMarshal(message)
	if marshalErr != nil {
		return marshalErr
	}

	bodyReader := bytes.NewReader(compressGzip(o.gzipWriter, marshalledBytes))
	postReq, reqErr := http.NewRequestWithContext(
		ctx,
		o.cfg.Method,
		o.cfg.OtelUrl.String(),
		bodyReader)
	if reqErr != nil {
		return reqErr
	}

	postReq.Header.Set("User-Agent", o.cfg.UserAgent)
	postReq.Header.Set("Content-Encoding", "gzip")
	postReq.Header.Set("Content-Type", "application/x-protobuf")
	postReq.ContentLength = -1

	response, respErr := o.client.Do(postReq)
	if respErr != nil {
		return respErr
	}
	defer response.Body.Close()

	if sc := response.StatusCode; sc >= 200 && sc <= 299 {
		return nil
	}
	return fmt.Errorf("received unexpected status code: %d for req %s %s", response.StatusCode, postReq.Method, postReq.URL.String())
}

func toMetrics(now time.Time, check servicelevels.Check) []*metricspb.Metric {
	var metrics []*metricspb.Metric
	for _, slo := range check.SLO {

		attributes := []*commonpb.KeyValue{
			StringAttribute("integration_id", string(check.IntegrationID)),
			StringAttribute("metric", slo.Metric.Name),
			BoolAttribute("breached", slo.Breach != nil),
		}
		for key, val := range slo.Tags {
			attributes = append(attributes, StringAttribute(key, val))
		}

		metricID := fmt.Sprintf("service_levels_%s", slo.Namespace)
		metricIDEvents := metricID + "_events"

		metrics = append(metrics, &metricspb.Metric{
			Name: metricID,
			Unit: slo.Metric.Unit,
			Data: &metricspb.Metric_Gauge{
				Gauge: &metricspb.Gauge{
					DataPoints: []*metricspb.NumberDataPoint{
						{
							Attributes:   attributes,
							TimeUnixNano: uint64(now.UnixNano()),
							Value: &metricspb.NumberDataPoint_AsDouble{
								AsDouble: slo.Metric.Value,
							},
						},
					},
				},
			},
		}, &metricspb.Metric{
			Name: metricIDEvents,
			Unit: "#",
			Data: &metricspb.Metric_Sum{
				Sum: &metricspb.Sum{
					DataPoints: []*metricspb.NumberDataPoint{
						{
							Attributes:   attributes,
							TimeUnixNano: uint64(now.UnixNano()),
							Value: &metricspb.NumberDataPoint_AsInt{
								AsInt: slo.Metric.EventsCount,
							},
						},
					},
					AggregationTemporality: metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA,
				},
			},
		})

		breakDownMetricID := metricID + "_breakdown"
		breakDownCountID := breakDownMetricID + "_events"

		for _, breakdown := range slo.Breakdown {
			attributes = []*commonpb.KeyValue{
				StringAttribute("integration_id", string(check.IntegrationID)),
				StringAttribute("breakdown", breakdown.Name),
				StringAttribute("metric", slo.Metric.Name),
				BoolAttribute("breached", slo.Breach != nil),
			}
			for key, val := range slo.Tags {
				attributes = append(attributes, StringAttribute(key, val))
			}

			metrics = append(metrics, &metricspb.Metric{
				Name: breakDownMetricID,
				Unit: breakdown.Unit,
				Data: &metricspb.Metric_Gauge{
					Gauge: &metricspb.Gauge{
						DataPoints: []*metricspb.NumberDataPoint{
							{
								Attributes:   attributes,
								TimeUnixNano: uint64(now.UnixNano()),
								Value: &metricspb.NumberDataPoint_AsDouble{
									AsDouble: breakdown.Value,
								},
							},
						},
					},
				},
			}, &metricspb.Metric{
				Name: breakDownCountID,
				Unit: "#",
				Data: &metricspb.Metric_Sum{
					Sum: &metricspb.Sum{
						DataPoints: []*metricspb.NumberDataPoint{
							{
								Attributes:   attributes,
								TimeUnixNano: uint64(now.UnixNano()),
								Value: &metricspb.NumberDataPoint_AsInt{
									AsInt: breakdown.EventsCount,
								},
							},
						},
						AggregationTemporality: metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA,
					},
				},
			})
		}
	}

	return metrics
}

func StringAttribute(key string, value string) *commonpb.KeyValue {
	return &commonpb.KeyValue{
		Key: key,
		Value: &commonpb.AnyValue{
			Value: &commonpb.AnyValue_StringValue{StringValue: value},
		},
	}
}

func BoolAttribute(key string, value bool) *commonpb.KeyValue {
	return &commonpb.KeyValue{
		Key: key,
		Value: &commonpb.AnyValue{
			Value: &commonpb.AnyValue_BoolValue{BoolValue: value},
		},
	}
}

func DefaultOtelHttpClient(sleep func(t time.Duration)) *http.Client {
	transport := &http.Transport{}
	roundTripper := http2.WrapWithRetries(
		transport,
		http2.RetryStatusCodes(
			http.StatusTooManyRequests,
			http.StatusBadGateway,
			http.StatusServiceUnavailable,
			http.StatusGatewayTimeout,
		),
		5,
		1.2,
		sleep)

	client := &http.Client{
		Timeout:   5 * time.Second,
		Transport: roundTripper,
	}
	return client
}

func compressGzip(gzip *gzip.Writer, in []byte) []byte {
	var compressedBytes bytes.Buffer
	gzip.Reset(&compressedBytes)
	_, _ = gzip.Write(in)
	_ = gzip.Close()
	return compressedBytes.Bytes()
}
