package otel

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"hotline/clock"
	"hotline/ingestions"
	"net/http/httptest"
	"strconv"
)

var _ = Describe("Envoy Ingestion of Traces", func() {
	s := envoySut{}

	It("ingests request from simple trace", func() {
		s.forHttpIngestion()
		s.requestWithSimpleTrace()
		requests := s.ingest()
		Expect(requests).To(HaveLen(1))
		Expect(requests[0]).To(Equal(&ingestions.HttpRequest{
			ID:              "5B8EFFF798038103D269B633813FC60C0:EEE19B7EC3C1B1740",
			IntegrationID:   "integration_id",
			ProtocolVersion: "1.1",
			Method:          "POST",
			StatusCode:      "200",
			URL:             newUrl("https://integration.com/order/123?param1=value1"),
			StartTime:       clock.ParseTime("2018-12-13T14:51:00Z"),
			EndTime:         clock.ParseTime("2018-12-13T14:51:01Z"),
			CorrelationID:   "req-id-value",
		}))
	})

	It("skips trace if http method not present", func() {
		s.forHttpIngestion()
		s.requestWithoutHttpMethod()
		requests := s.ingest()
		Expect(requests).To(HaveLen(0))
	})

	It("skips trace if status code not present", func() {
		s.forHttpIngestion()
		s.requestWithoutStatusCode()
		requests := s.ingest()
		Expect(requests).To(HaveLen(0))
	})

	It("skips trace if url not present", func() {
		s.forHttpIngestion()
		s.requestWithoutFullUrl()
		requests := s.ingest()
		Expect(requests).To(HaveLen(0))
	})

	It("skips trace if url not present", func() {
		s.forHttpIngestion()
		s.requestWithUnparseableFullUrl()
		requests := s.ingest()
		Expect(requests).To(HaveLen(0))
	})
})

type envoySut struct {
	server   *httptest.Server
	handler  *TracesHandler
	requests []*ingestions.HttpRequest
}

func (s *envoySut) forHttpIngestion() {
	s.requests = nil
	converter := NewProtoConverter()
	s.handler = NewTracesHandler(func(requests []*ingestions.HttpRequest) {
		s.requests = append(s.requests, requests...)
	}, converter)
	s.server = httptest.NewServer(s.handler)
}

func (s *envoySut) ingest() []*ingestions.HttpRequest {
	return s.requests
}

func (s *envoySut) requestWithSimpleTrace() {
	s.requestWitMultiResourceMultipleSpansWithModifier(1, 1, func(span *tracepb.Span) {})
}

func (s *envoySut) requestWithoutStatusCode() {
	s.requestWitMultiResourceMultipleSpansWithModifier(1, 1, func(span *tracepb.Span) {
		span.Attributes = remove(span.Attributes, EnvoyMappingNames.HttpStatusCode)
	})
}

func (s *envoySut) requestWithoutHttpMethod() {
	s.requestWitMultiResourceMultipleSpansWithModifier(1, 1, func(span *tracepb.Span) {
		span.Attributes = remove(span.Attributes, EnvoyMappingNames.HttpRequestMethod)
	})
}

func (s *envoySut) requestWithoutFullUrl() {
	s.requestWitMultiResourceMultipleSpansWithModifier(1, 1, func(span *tracepb.Span) {
		span.Attributes = remove(span.Attributes, EnvoyMappingNames.UrlFull)
	})
}

func (s *envoySut) requestWithUnparseableFullUrl() {
	s.requestWitMultiResourceMultipleSpansWithModifier(1, 1, func(span *tracepb.Span) {
		span.Attributes = remove(span.Attributes, EnvoyMappingNames.UrlFull)
		span.Attributes = append(span.Attributes, &commonpb.KeyValue{
			Key:   EnvoyMappingNames.UrlFull,
			Value: stringValue("%a"),
		})
	})
}

func (s *envoySut) requestWitMultiResourceMultipleSpansWithModifier(resourceCount int, traceCount int, modifier func(span *tracepb.Span)) {
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
						Key:   EnvoyMappingNames.HttpRequestMethod,
						Value: stringValue("POST"),
					},
					{
						Key:   EnvoyMappingNames.NetworkProtocolVersion,
						Value: stringValue("1.1"),
					},
					{
						Key:   EnvoyMappingNames.UrlFull,
						Value: stringValue("https://integration.com/order/123?param1=value1"),
					},
					{
						Key:   EnvoyMappingNames.HttpStatusCode,
						Value: stringValue("200"),
					},
					{
						Key:   EnvoyMappingNames.CorrelationID,
						Value: stringValue("req-id-value"),
					},
					{
						Key:   EnvoyMappingNames.IntegrationID,
						Value: stringValue("integration_id"),
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
						Name:    "envoy",
						Version: "2135e1a42f002a939d60581096291acb6abce695/1.33.2/Clean/RELEASE/BoringSSL",
					},
					Spans: spans,
				},
			},
			Resource: &resourcepb.Resource{
				Attributes: []*commonpb.KeyValue{
					{
						Key:   "service.name",
						Value: stringValue("external-integrations-proxy"),
					},
					{
						Key:   "telemetry.sdk.language",
						Value: stringValue("cpp"),
					},
					{
						Key:   "telemetry.sdk.name",
						Value: stringValue("envoy"),
					},
					{
						Key:   "telemetry.sdk.version",
						Value: stringValue("2135e1a42f002a939d60581096291acb6abce695/1.33.2/Clean/RELEASE/BoringSSL"),
					},
				},
			},
		})
	}
	sendTraces(s.server.URL, &coltracepb.ExportTraceServiceRequest{
		ResourceSpans: resourceSpans,
	})
}
