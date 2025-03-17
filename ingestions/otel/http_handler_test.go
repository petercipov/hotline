package otel

import (
	"bytes"
	"encoding/json"
	"errors"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"hotline/ingestions"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"time"
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
		Expect(requests[0]).To(Equal(ingestions.HttpRequest{
			ID:              "5B8EFFF798038103D269B633813FC60C0:EEE19B7EC3C1B1740",
			IntegrationID:   "integration.com",
			ProtocolVersion: "1.1",
			Method:          "POST",
			StatusCode:      "200",
			URL:             newUrl("https://integration.com/order/123?param1=value1"),
			StartTime:       parseTime("2018-12-13T14:51:00Z"),
			EndTime:         parseTime("2018-12-13T14:51:01Z"),
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

		Expect(requests[0].IntegrationID).To(Equal("id.of.integration"))
	})
})

type otelSut struct {
	server    *httptest.Server
	handler   *TracesHandler
	ingestion *FakeIngestion
}

func (s *otelSut) forHttpIngestion() {
	s.ingestion = &FakeIngestion{}
	s.handler = NewTracesHandler(s.ingestion, DefaultAttributeNames)
	s.server = httptest.NewServer(s.handler)
}

func (s *otelSut) requestWithEmptyTraces() {
	s.sendTraces(TracesMessage{
		ResourceSpans: []ResourceSpan{
			{
				ScopeSpans: []ScopeSpan{
					{
						Scope: Scope{
							Name:       "otel",
							Version:    "123",
							Attributes: []Attribute{},
						},
						Spans: nil,
					},
				},
			},
		},
	})
}

func (s *otelSut) sendTraces(message TracesMessage) {
	raw, marshalErr := json.Marshal(message)
	Expect(marshalErr).ToNot(HaveOccurred())
	// https://github.com/open-telemetry/opentelemetry-collector/blob/432d92d8b366f6831323a928783f1ed867c42050/exporter/otlphttpexporter/otlp.go#L185
	req, createErr := http.NewRequest(http.MethodPost, s.server.URL, bytes.NewReader(raw))
	Expect(createErr).ToNot(HaveOccurred())
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "remote-service-otel-exporter")
	resp, reqErr := http.DefaultClient.Do(req)
	Expect(reqErr).ToNot(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))
}

func (s *otelSut) ingest() []ingestions.HttpRequest {
	return s.ingestion.requests
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
	s.requestWitMultiResourceMultipleSpansWithModifier(resourceCount, traceCount, func(span *Span) {})
}

func (s *otelSut) requestWitMultiResourceMultipleSpansWithModifier(resourceCount int, traceCount int, modifier func(span *Span)) {
	var resourceSpans []ResourceSpan
	for ri := 0; ri < resourceCount; ri++ {
		var spans []Span
		for ti := 0; ti < traceCount; ti++ {
			span := Span{
				TraceId:           "5B8EFFF798038103D269B633813FC60C" + strconv.Itoa(ri),
				SpanId:            "EEE19B7EC3C1B174" + strconv.Itoa(ti),
				ParentSpanId:      "EEE19B7EC3C1B173",
				Name:              "request to remote integration",
				StartTimeUnixNano: "1544712660000000000",
				EndTimeUnixNano:   "1544712661000000000",
				Kind:              3,
				// https://opentelemetry.io/docs/specs/semconv/http/http-spans/
				Attributes: []Attribute{
					{
						Key: "http.request.method",
						Value: StringValue{
							StringValue: "POST",
						},
					},
					{
						Key: "network.protocol.version",
						Value: StringValue{
							StringValue: "1.1",
						},
					},
					{
						Key: "url.full",
						Value: StringValue{
							StringValue: "https://integration.com/order/123?param1=value1",
						},
					},
					{
						Key: "http.response.status_code",
						Value: StringValue{
							StringValue: "200",
						},
					},
				},
			}
			modifier(&span)
			spans = append(spans, span)
		}

		resourceSpans = append(resourceSpans, ResourceSpan{
			ScopeSpans: []ScopeSpan{
				{
					Scope: Scope{
						Name:    "otel",
						Version: "123",
					},
					Spans: spans,
				},
			},
		})
	}
	s.sendTraces(TracesMessage{
		ResourceSpans: resourceSpans,
	})
}

func (s *otelSut) requestWithSimpleTraceWithIntegrationID(integrationID string) {
	s.requestWitMultiResourceMultipleSpansWithModifier(1, 1, func(span *Span) {
		span.Attributes = append(span.Attributes, Attribute{
			Key: "integration.id",
			Value: StringValue{
				StringValue: integrationID,
			},
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

type FakeIngestion struct {
	requests []ingestions.HttpRequest
}

func (f *FakeIngestion) Ingest(requests []ingestions.HttpRequest) {
	f.requests = append(f.requests, requests...)
}

func newUrl(s string) *url.URL {
	parsedUrl, parseErr := url.Parse(s)
	Expect(parseErr).ToNot(HaveOccurred())
	return parsedUrl
}

func parseTime(nowString string) time.Time {
	now, parseErr := time.Parse(time.RFC3339, nowString)
	Expect(parseErr).NotTo(HaveOccurred())
	return now
}

type fakeErrReader int

func (fakeErrReader) Read(_ []byte) (n int, err error) {
	return 0, errors.New("test error")
}
