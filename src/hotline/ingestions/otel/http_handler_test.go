package otel

import (
	"bytes"
	"errors"
	"hotline/clock"
	"hotline/ingestions"
	"hotline/integrations"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

var _ = Describe("Otel Http Ingestion of Traces", func() {
	s := otelSut{}

	AfterEach(func() {
		s.Close()
	})

	It("returns 500 for malformed it", func() {
		s.forHttpIngestion()
		status := s.requestWithMalformedIO()
		Expect(status).To(Equal(http.StatusInternalServerError))
	})

	It("returns 400 invalid request body", func() {
		s.forHttpIngestion()
		status := s.requestWithInvalidBody()
		Expect(status).To(Equal(http.StatusBadRequest))
	})

	It("ingests nothing for no traces in request", func() {
		s.forHttpIngestion()
		s.requestWithEmptyTraces()
		requests := s.ingest()
		Expect(requests).To(HaveLen(0))
	})

	It("ingests request from simple trace", func() {
		s.forHttpIngestion()
		s.requestWithSimpleTrace()
		requests := s.ingest()
		Expect(requests).To(HaveLen(1))
		Expect(requests[0]).To(Equal(&ingestions.HttpRequest{
			ID:              "5B8EFFF798038103D269B633813FC60C0:EEE19B7EC3C1B1740",
			IntegrationID:   "integration.com",
			ProtocolVersion: "1.1",
			Method:          "POST",
			StatusCode:      "200",
			URL:             newUrl("https://integration.com/order/123?param1=value1"),
			StartTime:       clock.ParseTime("2018-12-13T14:51:00Z"),
			EndTime:         clock.ParseTime("2018-12-13T14:51:01Z"),
		}))
	})

	It("ingests multiple traces from request", func() {
		s.forHttpIngestion()
		s.requestWithMultipleSpans(5)
		requests := s.ingest()
		Expect(requests).To(HaveLen(5))
	})

	It("ingests multiple traces from multi resource request", func() {
		s.forHttpIngestion()
		s.requestWitMultiResourceMultipleSpans(5, 5)
		requests := s.ingest()
		Expect(requests).To(HaveLen(25))
	})

	It("ingests simple request with integrationID", func() {
		s.forHttpIngestion()
		s.requestWithSimpleTraceWithIntegrationID("id.of.integration")
		requests := s.ingest()
		Expect(requests).To(HaveLen(1))

		Expect(requests[0].IntegrationID).To(Equal(integrations.ID("id.of.integration")))
	})

	It("refuses to ingest other than kind client spans", func() {
		s.forHttpIngestion()
		s.requestWithSimpleServerSpan()
		requests := s.ingest()
		Expect(requests).To(HaveLen(0))
	})

	It("ingest trace without status code but error type", func() {
		s.forHttpIngestion()
		s.requestWithErrorType("timeout")
		requests := s.ingest()
		Expect(requests).To(HaveLen(1))
		Expect(requests[0]).To(Equal(&ingestions.HttpRequest{
			ID:              "5B8EFFF798038103D269B633813FC60C0:EEE19B7EC3C1B1740",
			IntegrationID:   "integration.com",
			ProtocolVersion: "1.1",
			Method:          "POST",
			ErrorType:       "timeout",
			URL:             newUrl("https://integration.com/order/123?param1=value1"),
			StartTime:       clock.ParseTime("2018-12-13T14:51:00Z"),
			EndTime:         clock.ParseTime("2018-12-13T14:51:01Z"),
		}))
	})

	It("ingests minimal http trace", func() {
		s.forHttpIngestion()
		s.requestWithMinimalTrace()
		requests := s.ingest()
		Expect(requests).To(HaveLen(2))
		Expect(requests).To(Equal([]*ingestions.HttpRequest{
			{
				ID:            "5B8EFFF798038103D269B633813FC60C0:EEE19B7EC3C1B1740",
				IntegrationID: "integration.com",
				Method:        "GET",
				StatusCode:    "200",
				URL:           newUrl("https://integration.com/order/123?param1=value1"),
				StartTime:     clock.ParseTime("2018-12-13T14:51:00Z"),
				EndTime:       clock.ParseTime("2018-12-13T14:51:01Z"),
				CorrelationID: "req-id-value",
			},
			{
				ID:            "5B8EFFF798038103D269B633813FC60C0:EEE19B7EC3C1B1740",
				IntegrationID: "integration.com",
				Method:        "POST",
				ErrorType:     "timeout",
				URL:           newUrl("https://integration.com/order/123?param1=value1"),
				StartTime:     clock.ParseTime("2018-12-13T14:51:00Z"),
				EndTime:       clock.ParseTime("2018-12-13T14:51:01Z"),
			},
		}))
	})

	It("skips trace if no http method not present", func() {
		s.forHttpIngestion()
		s.requestWithoutHttpMethod()
		requests := s.ingest()
		Expect(requests).To(HaveLen(0))
	})

	It("skips trace if no full url not present", func() {
		s.forHttpIngestion()
		s.requestWithoutFullUrl()
		requests := s.ingest()
		Expect(requests).To(HaveLen(0))
	})

	It("skips trace if full url not parseable", func() {
		s.forHttpIngestion()
		s.requestWithUnparseableFullUrl()
		requests := s.ingest()
		Expect(requests).To(HaveLen(0))
	})

	It("skips trace if integration id is empty", func() {
		s.forHttpIngestion()
		s.requestWithIntegrationIDEmpty()
		requests := s.ingest()
		Expect(requests).To(HaveLen(0))
	})

	It("skips trace if no status and no error type", func() {
		s.forHttpIngestion()
		s.requestWithNoStatusNoErrorType()
		requests := s.ingest()
		Expect(requests).To(HaveLen(0))
	})
})

type otelSut struct {
	server   *httptest.Server
	handler  *TracesHandler
	requests []*ingestions.HttpRequest
}

func (s *otelSut) forHttpIngestion() {
	s.requests = nil
	converter := NewProtoConverter()
	s.handler = NewTracesHandler(func(requests []*ingestions.HttpRequest) {
		s.requests = append(s.requests, requests...)
	}, converter)
	s.server = httptest.NewServer(s.handler)
}

func (s *otelSut) requestWithEmptyTraces() {
	message := &coltracepb.ExportTraceServiceRequest{
		ResourceSpans: []*tracepb.ResourceSpans{
			{
				ScopeSpans: []*tracepb.ScopeSpans{
					{
						Scope: &commonpb.InstrumentationScope{
							Name:    "otel",
							Version: "123",
						},
					},
				},
			},
		},
	}
	sendTraces(s.server.URL, message)
}

func sendTraces(url string, message *coltracepb.ExportTraceServiceRequest) {
	raw, marshalErr := proto.Marshal(message)
	Expect(marshalErr).ToNot(HaveOccurred())
	// https://github.com/open-telemetry/opentelemetry-collector/blob/432d92d8b366f6831323a928783f1ed867c42050/exporter/otlphttpexporter/otlp.go#L185
	req, createErr := http.NewRequest(http.MethodPost, url, bytes.NewReader(raw))
	Expect(createErr).ToNot(HaveOccurred())
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "remote-service-otel-exporter")
	resp, reqErr := http.DefaultClient.Do(req)
	Expect(reqErr).ToNot(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))
}

func (s *otelSut) ingest() []*ingestions.HttpRequest {
	return s.requests
}

func (s *otelSut) Close() {
	s.server.Close()
}

func (s *otelSut) requestWithSimpleTrace() {
	s.requestWitMultiResourceMultipleSpans(1, 1)
}

func (s *otelSut) requestWithMultipleSpans(count int) {
	s.requestWitMultiResourceMultipleSpans(1, count)
}

func (s *otelSut) requestWitMultiResourceMultipleSpans(resourceCount int, traceCount int) {
	s.requestWitMultiResourceMultipleSpansWithModifier(resourceCount, traceCount, func(span *tracepb.Span) {})
}

func (s *otelSut) requestWitMultiResourceMultipleSpansWithModifier(resourceCount int, traceCount int, modifier func(span *tracepb.Span)) {
	var resourceSpans []*tracepb.ResourceSpans
	for ri := 0; ri < resourceCount; ri++ {
		var spans []*tracepb.Span
		for ti := 0; ti < traceCount; ti++ {
			span := &tracepb.Span{
				TraceId:           []byte("5B8EFFF798038103D269B633813FC60C" + strconv.Itoa(ri)),
				SpanId:            []byte("EEE19B7EC3C1B174" + strconv.Itoa(ti)),
				ParentSpanId:      []byte("EEE19B7EC3C1B173"),
				Name:              "request to remote integration",
				StartTimeUnixNano: 1544712660000000000,
				EndTimeUnixNano:   1544712661000000000,
				Kind:              3,
				// https://opentelemetry.io/docs/specs/semconv/http/http-spans/
				Attributes: []*commonpb.KeyValue{
					{
						Key:   StandardMappingNames.HttpRequestMethod,
						Value: stringValue("POST"),
					},
					{
						Key:   StandardMappingNames.NetworkProtocolVersion,
						Value: stringValue("1.1"),
					},
					{
						Key:   StandardMappingNames.UrlFull,
						Value: stringValue("https://integration.com/order/123?param1=value1"),
					},
					{
						Key:   StandardMappingNames.HttpStatusCode,
						Value: stringValue("200"),
					},
				},
			}
			modifier(span)
			spans = append(spans, span)
		}

		resourceSpans = append(resourceSpans, &tracepb.ResourceSpans{
			ScopeSpans: []*tracepb.ScopeSpans{
				{
					Scope: &commonpb.InstrumentationScope{
						Name:    "otel",
						Version: "123",
					},
					Spans: spans,
				},
			},
		})
	}
	sendTraces(s.server.URL, &coltracepb.ExportTraceServiceRequest{
		ResourceSpans: resourceSpans,
	})
}

func stringValue(value string) *commonpb.AnyValue {
	return &commonpb.AnyValue{
		Value: &commonpb.AnyValue_StringValue{StringValue: value},
	}
}

func (s *otelSut) requestWithSimpleTraceWithIntegrationID(integrationID string) {
	s.requestWitMultiResourceMultipleSpansWithModifier(1, 1, func(span *tracepb.Span) {
		span.Attributes = append(span.Attributes, &commonpb.KeyValue{
			Key:   StandardMappingNames.IntegrationID,
			Value: stringValue(integrationID),
		})
	})
}

func (s *otelSut) requestWithMalformedIO() int {
	req, createErr := http.NewRequest(http.MethodPost, s.server.URL, fakeErrReader(1))
	Expect(createErr).ToNot(HaveOccurred())

	recorder := httptest.NewRecorder()
	s.handler.ServeHTTP(recorder, req)

	return recorder.Code
}

func (s *otelSut) requestWithInvalidBody() int {
	req, createErr := http.NewRequest(http.MethodPost, s.server.URL, bytes.NewReader([]byte("invalid json body")))
	Expect(createErr).ToNot(HaveOccurred())
	resp, reqErr := http.DefaultClient.Do(req)
	Expect(reqErr).ToNot(HaveOccurred())
	return resp.StatusCode
}

func (s *otelSut) requestWithSimpleServerSpan() {
	s.requestWitMultiResourceMultipleSpansWithModifier(1, 1, func(span *tracepb.Span) {
		span.Kind = tracepb.Span_SPAN_KIND_SERVER
	})
}

func (s *otelSut) requestWithErrorType(errorType string) {
	s.requestWitMultiResourceMultipleSpansWithModifier(1, 1, func(span *tracepb.Span) {
		span.Attributes = remove(span.Attributes, StandardMappingNames.HttpStatusCode)
		span.Attributes = append(span.Attributes, &commonpb.KeyValue{
			Key:   StandardMappingNames.ErrorType,
			Value: stringValue(errorType),
		})
	})
}

func (s *otelSut) requestWithMinimalTrace() {
	s.requestWitMultiResourceMultipleSpansWithModifier(1, 1, func(span *tracepb.Span) {
		span.Kind = tracepb.Span_SPAN_KIND_CLIENT
		span.Attributes = []*commonpb.KeyValue{
			{
				Key:   StandardMappingNames.HttpRequestMethod,
				Value: stringValue("GET"),
			},
			{
				Key:   StandardMappingNames.HttpStatusCode,
				Value: stringValue("200"),
			},
			{
				Key:   StandardMappingNames.UrlFull,
				Value: stringValue("https://integration.com/order/123?param1=value1"),
			},
			{
				Key:   StandardMappingNames.CorrelationID,
				Value: stringValue("req-id-value"),
			},
		}
	})

	s.requestWitMultiResourceMultipleSpansWithModifier(1, 1, func(span *tracepb.Span) {
		span.Kind = tracepb.Span_SPAN_KIND_CLIENT
		span.Attributes = []*commonpb.KeyValue{
			{
				Key:   StandardMappingNames.HttpRequestMethod,
				Value: stringValue("POST"),
			},
			{
				Key:   StandardMappingNames.ErrorType,
				Value: stringValue("timeout"),
			},
			{
				Key:   StandardMappingNames.UrlFull,
				Value: stringValue("https://integration.com/order/123?param1=value1"),
			},
		}
	})
}

func (s *otelSut) requestWithoutHttpMethod() {
	s.requestWitMultiResourceMultipleSpansWithModifier(1, 1, func(span *tracepb.Span) {
		span.Attributes = remove(span.Attributes, StandardMappingNames.HttpRequestMethod)
	})
}

func (s *otelSut) requestWithoutFullUrl() {
	s.requestWitMultiResourceMultipleSpansWithModifier(1, 1, func(span *tracepb.Span) {
		span.Attributes = remove(span.Attributes, StandardMappingNames.UrlFull)
	})
}

func (s *otelSut) requestWithUnparseableFullUrl() {
	s.requestWitMultiResourceMultipleSpansWithModifier(1, 1, func(span *tracepb.Span) {
		span.Attributes = remove(span.Attributes, StandardMappingNames.UrlFull)
		span.Attributes = append(span.Attributes, &commonpb.KeyValue{
			Key:   StandardMappingNames.UrlFull,
			Value: stringValue("%a"),
		})
	})
}

func (s *otelSut) requestWithIntegrationIDEmpty() {
	s.requestWitMultiResourceMultipleSpansWithModifier(1, 1, func(span *tracepb.Span) {
		span.Attributes = remove(span.Attributes, StandardMappingNames.IntegrationID)
		span.Attributes = append(span.Attributes, &commonpb.KeyValue{
			Key:   StandardMappingNames.IntegrationID,
			Value: stringValue(""),
		})
	})
}

func (s *otelSut) requestWithNoStatusNoErrorType() {
	s.requestWitMultiResourceMultipleSpansWithModifier(1, 1, func(span *tracepb.Span) {
		span.Attributes = remove(span.Attributes, StandardMappingNames.HttpStatusCode)
		span.Attributes = remove(span.Attributes, StandardMappingNames.ErrorType)
	})
}

func newUrl(s string) *url.URL {
	parsedUrl, parseErr := url.Parse(s)
	Expect(parseErr).ToNot(HaveOccurred())
	return parsedUrl
}

type fakeErrReader int

func (fakeErrReader) Read(_ []byte) (n int, err error) {
	return 0, errors.New("test error")
}

func remove(l []*commonpb.KeyValue, name string) []*commonpb.KeyValue {
	returnList := l
	for i, attr := range l {
		if attr.Key == name {
			l[i] = l[len(l)-1]
			returnList = l[:len(l)-1]
			break
		}
	}
	return returnList
}
