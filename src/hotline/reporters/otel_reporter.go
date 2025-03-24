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
	otelUrl, parseErr := url.ParseRequestURI(fmt.Sprintf("%s://%s/v1/reporters", scheme, host))
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
	return fmt.Errorf("received unexpected status code: %d", response.StatusCode)
}

func toMetrics(now time.Time, check servicelevels.Check) []*metricspb.Metric {
	var metrics []*metricspb.Metric
	for _, slo := range check.SLO {

		attributes := []*commonpb.KeyValue{
			stringAttribute("integration_id", string(check.IntegrationID)),
			boolAttribute("breached", slo.Breach != nil),
		}
		for key, val := range slo.Tags {
			attributes = append(attributes, stringAttribute(key, val))
		}

		metricID := fmt.Sprintf("service_levels_%s_%s", slo.Namespace, slo.Metric.Name)

		metrics = append(metrics, &metricspb.Metric{
			Name: metricID,
			Unit: "percent",
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
		})

		for _, breakdown := range slo.Breakdown {
			attributes = []*commonpb.KeyValue{
				stringAttribute("integration_id", string(check.IntegrationID)),
				stringAttribute("breakdown", breakdown.Name),
			}
			for key, val := range slo.Tags {
				attributes = append(attributes, stringAttribute(key, val))
			}

			metrics = append(metrics, &metricspb.Metric{
				Name: metricID,
				Unit: "percent",
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
			})
		}
	}

	return metrics
}

func stringAttribute(key string, value string) *commonpb.KeyValue {
	return &commonpb.KeyValue{
		Key: key,
		Value: &commonpb.AnyValue{
			Value: &commonpb.AnyValue_StringValue{StringValue: value},
		},
	}
}

func boolAttribute(key string, value bool) *commonpb.KeyValue {
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
