package reporters_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	"google.golang.org/protobuf/proto"
	"hotline/reporters"
	"hotline/servicelevels"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"time"
)

var _ = Describe("OTEL Reporter", func() {
	sut := otelReporterSUT{}
	AfterEach(sut.Close)

	It("should ommit send call for empty reporters", func() {
		sut.forReporter()
		sut.sendEmptyMetrics()
		messages := sut.receivedOtelMessages()
		Expect(messages).To(HaveLen(0))
	})

	It("should send a message for SLO check", func() {
		sut.forReporter()
		err := sut.sendSloCheck()
		Expect(err).To(BeNil())
		messages := sut.receivedOtelMessages()
		Expect(messages).To(HaveLen(1))
		Expect(messages[0].ResourceMetrics).To(HaveLen(1))
		Expect(messages[0].ResourceMetrics[0].ScopeMetrics).To(HaveLen(1))
		Expect(messages[0].ResourceMetrics[0].ScopeMetrics[0].Metrics).To(HaveLen(6))
		Expect(messages[0].ResourceMetrics[0].ScopeMetrics[0].Metrics[0]).To(Equal(&metricspb.Metric{
			Name: "service_levels_http_route_status",
			Unit: "%",
			Data: &metricspb.Metric_Gauge{
				Gauge: &metricspb.Gauge{
					DataPoints: []*metricspb.NumberDataPoint{
						{
							Attributes: []*commonpb.KeyValue{
								reporters.StringAttribute("integration_id", "integration-abcd"),
								reporters.StringAttribute("metric", "unexpected"),
								reporters.BoolAttribute("breached", true),
								reporters.StringAttribute("http_route", "iam.example.com/users"),
							},
							TimeUnixNano: 1740225845000000000,
							Value: &metricspb.NumberDataPoint_AsDouble{
								AsDouble: 100,
							},
						},
					},
				},
			},
		}))
		Expect(messages[0].ResourceMetrics[0].ScopeMetrics[0].Metrics[1]).To(Equal(&metricspb.Metric{
			Name: "service_levels_http_route_status_events",
			Unit: "#",
			Data: &metricspb.Metric_Sum{
				Sum: &metricspb.Sum{
					AggregationTemporality: 1,
					DataPoints: []*metricspb.NumberDataPoint{
						{
							Attributes: []*commonpb.KeyValue{
								reporters.StringAttribute("integration_id", "integration-abcd"),
								reporters.StringAttribute("metric", "unexpected"),
								reporters.BoolAttribute("breached", true),
								reporters.StringAttribute("http_route", "iam.example.com/users"),
							},
							TimeUnixNano: 1740225845000000000,
							Value: &metricspb.NumberDataPoint_AsInt{
								AsInt: 2,
							},
						},
					},
				},
			},
		}))
		Expect(messages[0].ResourceMetrics[0].ScopeMetrics[0].Metrics[2]).To(Equal(&metricspb.Metric{
			Name: "service_levels_http_route_status_breakdown",
			Unit: "%",
			Data: &metricspb.Metric_Gauge{
				Gauge: &metricspb.Gauge{
					DataPoints: []*metricspb.NumberDataPoint{
						{
							Attributes: []*commonpb.KeyValue{
								reporters.StringAttribute("integration_id", "integration-abcd"),
								reporters.StringAttribute("breakdown", "4xx"),
								reporters.StringAttribute("metric", "unexpected"),
								reporters.BoolAttribute("breached", true),
								reporters.StringAttribute("http_route", "iam.example.com/users"),
							},
							TimeUnixNano: 1740225845000000000,
							Value: &metricspb.NumberDataPoint_AsDouble{
								AsDouble: 50,
							},
						},
					},
				},
			},
		}))

		Expect(messages[0].ResourceMetrics[0].ScopeMetrics[0].Metrics[3]).To(Equal(&metricspb.Metric{
			Name: "service_levels_http_route_status_breakdown_events",
			Unit: "#",
			Data: &metricspb.Metric_Sum{
				Sum: &metricspb.Sum{
					AggregationTemporality: 1,
					DataPoints: []*metricspb.NumberDataPoint{
						{
							Attributes: []*commonpb.KeyValue{
								reporters.StringAttribute("integration_id", "integration-abcd"),
								reporters.StringAttribute("breakdown", "4xx"),
								reporters.StringAttribute("metric", "unexpected"),
								reporters.BoolAttribute("breached", true),
								reporters.StringAttribute("http_route", "iam.example.com/users"),
							},
							TimeUnixNano: 1740225845000000000,
							Value: &metricspb.NumberDataPoint_AsInt{
								AsInt: 1,
							},
						},
					},
				},
			},
		}))

		Expect(messages[0].ResourceMetrics[0].ScopeMetrics[0].Metrics[4]).To(Equal(&metricspb.Metric{
			Name: "service_levels_http_route_status_breakdown",
			Unit: "%",
			Data: &metricspb.Metric_Gauge{
				Gauge: &metricspb.Gauge{
					DataPoints: []*metricspb.NumberDataPoint{
						{
							Attributes: []*commonpb.KeyValue{
								reporters.StringAttribute("integration_id", "integration-abcd"),
								reporters.StringAttribute("breakdown", "5xx"),
								reporters.StringAttribute("metric", "unexpected"),
								reporters.BoolAttribute("breached", true),
								reporters.StringAttribute("http_route", "iam.example.com/users"),
							},
							TimeUnixNano: 1740225845000000000,
							Value: &metricspb.NumberDataPoint_AsDouble{
								AsDouble: 50,
							},
						},
					},
				},
			},
		}))

		Expect(messages[0].ResourceMetrics[0].ScopeMetrics[0].Metrics[5]).To(Equal(&metricspb.Metric{
			Name: "service_levels_http_route_status_breakdown_events",
			Unit: "#",
			Data: &metricspb.Metric_Sum{
				Sum: &metricspb.Sum{
					AggregationTemporality: 1,
					DataPoints: []*metricspb.NumberDataPoint{
						{
							Attributes: []*commonpb.KeyValue{
								reporters.StringAttribute("integration_id", "integration-abcd"),
								reporters.StringAttribute("breakdown", "5xx"),
								reporters.StringAttribute("metric", "unexpected"),
								reporters.BoolAttribute("breached", true),
								reporters.StringAttribute("http_route", "iam.example.com/users"),
							},
							TimeUnixNano: 1740225845000000000,
							Value: &metricspb.NumberDataPoint_AsInt{
								AsInt: 1,
							},
						},
					},
				},
			},
		}))
	})

	It("should fail sending message if message not marshalled", func() {
		sut.forReporter()
		err := sut.sendUnmarshalableMessage()
		Expect(err).To(HaveOccurred())
	})

	It("should fail sending message if wrong method", func() {
		sut.forReporter()
		err := sut.sendMesaageWithWrongMethod()
		Expect(err).To(HaveOccurred())
	})

	It("should fail sending message if network failed", func() {
		sut.forReporter()
		err := sut.sendMessageWithNetworkFailure()
		Expect(err).To(HaveOccurred())
	})

	It("should fail if reponse not 20x", func() {
		sut.forReporterWithFailingResponse(http.StatusNotFound)
		err := sut.sendSloCheck()
		Expect(err).To(HaveOccurred())
		messages := sut.receivedOtelMessages()
		Expect(messages).To(HaveLen(1))
	})

	It("should retry and fail if reponse not 20x", func() {
		sut.forReporterWithFailingResponse(http.StatusTooManyRequests)
		err := sut.sendSloCheck()
		Expect(err).To(HaveOccurred())
		messages := sut.receivedOtelMessages()
		Expect(messages).To(HaveLen(6))
	})

})

var _ = Describe("OTELUrl", func() {
	It("should return a valid unsecure URL", func() {
		otel, parseErr := reporters.NewOtelUrl(false, "localhost:4318")
		Expect(parseErr).To(BeNil())
		Expect(otel.String()).To(Equal("http://localhost:4318/v1/metrics"))
	})

	It("should return a valid secure URL", func() {
		otel, parseErr := reporters.NewOtelUrl(true, "localhost:4318")
		Expect(parseErr).To(BeNil())
		Expect(otel.String()).To(Equal("https://localhost:4318/v1/metrics"))
	})

	It("should fail if host invalid", func() {
		_, parseErr := reporters.NewOtelUrl(true, "[::")
		Expect(parseErr).NotTo(BeNil())
	})
})

type otelReporterSUT struct {
	cfg        *reporters.OtelReporterConfig
	reporter   *reporters.OtelReporter
	testServer *httptest.Server
	httpClient *http.Client

	marshalFunc func(proto.Message) ([]byte, error)

	statusCode int

	receivedMessages []*colmetricspb.ExportMetricsServiceRequest
}

func (r *otelReporterSUT) forReporter() {
	r.statusCode = http.StatusOK
	r.testServer = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {

		bodyBytes, bodyReadErr := uncompressGzip(request.Body)
		Expect(bodyReadErr).To(BeNil())

		message := &colmetricspb.ExportMetricsServiceRequest{}
		unmarshalErr := proto.Unmarshal(bodyBytes, message)
		if unmarshalErr != nil {
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		r.receivedMessages = append(r.receivedMessages, message)

		writer.WriteHeader(r.statusCode)
	}))

	testServerUrl, parseErr := url.Parse(r.testServer.URL)
	Expect(parseErr).NotTo(HaveOccurred())

	otelUrl, parseOtelErr := reporters.NewOtelUrl(false, testServerUrl.Host)
	Expect(parseOtelErr).NotTo(HaveOccurred())

	r.httpClient = reporters.DefaultOtelHttpClient(func(t time.Duration) {
		time.Sleep(1 * time.Microsecond)
	})
	gzipWriter := gzip.NewWriter(io.Discard)

	r.marshalFunc = proto.Marshal

	r.cfg = &reporters.OtelReporterConfig{
		OtelUrl:   otelUrl,
		UserAgent: "test-agent",
		Method:    http.MethodPost,
	}

	r.reporter = reporters.NewOtelReporter(
		r.cfg,
		r.httpClient,
		gzipWriter,
		func(m proto.Message) ([]byte, error) {
			return r.marshalFunc(m)
		})
}

func (r *otelReporterSUT) forReporterWithFailingResponse(statusCode int) {
	r.forReporter()
	r.statusCode = statusCode
}

func (r *otelReporterSUT) receivedOtelMessages() []*colmetricspb.ExportMetricsServiceRequest {
	return r.receivedMessages
}

func (r *otelReporterSUT) sendEmptyMetrics() {
	reportErr := r.reporter.ReportChecks(context.Background(), &servicelevels.CheckReport{
		Now: parseTime("2025-02-22T12:04:05Z"),
	})
	Expect(reportErr).NotTo(HaveOccurred())
}

func (r *otelReporterSUT) Close() {
	r.testServer.Close()
	r.receivedMessages = nil
}

func (r *otelReporterSUT) sendSloCheck() error {
	reportErr := r.reporter.ReportChecks(context.Background(), &servicelevels.CheckReport{
		Now:    parseTime("2025-02-22T12:04:05Z"),
		Checks: simpleSLOCheck(),
	})

	return reportErr
}

func simpleSLOCheck() []servicelevels.Check {
	return []servicelevels.Check{
		{
			IntegrationID: "integration-abcd",
			SLO: []servicelevels.SLOCheck{
				{
					Namespace: "http_route_status",
					Metric: servicelevels.Metric{
						Name:        "unexpected",
						Value:       100,
						Unit:        "%",
						EventsCount: 2,
					},
					Tags: map[string]string{
						"http_route": "iam.example.com/users",
					},
					Breakdown: []servicelevels.Metric{
						{
							Name:        "4xx",
							Value:       50,
							Unit:        "%",
							EventsCount: 1,
						},
						{
							Name:        "5xx",
							Value:       50,
							Unit:        "%",
							EventsCount: 1,
						},
					},
					Breach: &servicelevels.SLOBreach{
						ThresholdValue: 0.1,
						ThresholdUnit:  "%",
						Operation:      "<",
						WindowDuration: 1 * time.Hour,
					},
				},
			},
		},
	}
}

func (r *otelReporterSUT) sendUnmarshalableMessage() error {
	r.marshalFunc = func(message proto.Message) ([]byte, error) {
		return nil, errors.New("unexpected")
	}

	reportErr := r.reporter.ReportChecks(context.Background(), &servicelevels.CheckReport{
		Now:    parseTime("2025-02-22T12:04:05Z"),
		Checks: simpleSLOCheck(),
	})
	r.marshalFunc = proto.Marshal

	return reportErr
}

func (r *otelReporterSUT) sendMesaageWithWrongMethod() error {
	r.cfg.Method = "❌"
	reportErr := r.reporter.ReportChecks(context.Background(), &servicelevels.CheckReport{
		Now:    parseTime("2025-02-22T12:04:05Z"),
		Checks: simpleSLOCheck(),
	})
	r.cfg.Method = http.MethodPost

	return reportErr
}

func (r *otelReporterSUT) sendMessageWithNetworkFailure() interface{} {
	failing := &failingTransport{}
	previous := r.httpClient.Transport
	r.httpClient.Transport = failing
	reportErr := r.reporter.ReportChecks(context.Background(), &servicelevels.CheckReport{
		Now:    parseTime("2025-02-22T12:04:05Z"),
		Checks: simpleSLOCheck(),
	})
	r.httpClient.Transport = previous
	return reportErr
}

func parseTime(nowString string) time.Time {
	now, parseErr := time.Parse(time.RFC3339, nowString)
	Expect(parseErr).NotTo(HaveOccurred())
	return now
}

type failingTransport struct {
}

func (t *failingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return nil, errors.New("failing due to network")
}

func uncompressGzip(reader io.Reader) ([]byte, error) {
	decompressor, _ := gzip.NewReader(reader)
	var uncompressed bytes.Buffer
	_, err := io.Copy(&uncompressed, decompressor)
	if err != nil {
		return nil, err
	}
	_ = decompressor.Close()
	return uncompressed.Bytes(), nil
}
