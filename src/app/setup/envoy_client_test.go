package setup_test

import (
	"hotline/ingestions/otel"
	"math/rand"
	"strconv"
	"time"

	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

type EnvoyClient struct {
	URL string
}

func (s *EnvoyClient) SendSomeTraffic(now time.Time, integrationID string) (int, error) {
	r := rand.New(rand.NewSource(0))

	statusCodes := []string{
		"200", "201", "403", "500", "503",
	}

	return s.sendTraffic(integrationID, 10, 1000, func(span *tracepb.Span) {

		startTime := now
		endTime := startTime.Add(time.Duration(r.Intn(10000)) * time.Millisecond)

		span.EndTimeUnixNano = uint64(endTime.UnixNano())
		span.StartTimeUnixNano = uint64(startTime.UnixNano())

		statusCode := statusCodes[r.Intn(len(statusCodes))]
		span.Attributes = append(span.Attributes, &commonpb.KeyValue{
			Key:   otel.EnvoyMappingNames.HttpStatusCode,
			Value: stringValue(statusCode),
		})
	})
}

func (s *EnvoyClient) sendTraffic(integrationID string, resourceCount int, traceCount int, modifier func(span *tracepb.Span)) (int, error) {
	var resourceSpans []*tracepb.ResourceSpans
	for ri := 0; ri < resourceCount; ri++ {
		var spans []*tracepb.Span
		for ti := 0; ti < traceCount; ti++ {
			span := &tracepb.Span{
				TraceId:           []byte("5B8EFFF798038103D269B633813FC60C" + strconv.Itoa(ri)),
				SpanId:            []byte("EEE19B7EC3C1B174" + strconv.Itoa(ti)),
				ParentSpanId:      []byte("EEE19B7EC3C1B173" + strconv.Itoa(ri)),
				Name:              "request to remote integration",
				StartTimeUnixNano: 1544712660000000000,
				EndTimeUnixNano:   1544712661000000000,
				Kind:              2,
				// https://opentelemetry.io/docs/specs/semconv/http/http-spans/
				Attributes: []*commonpb.KeyValue{
					{
						Key:   otel.EnvoyMappingNames.HttpRequestMethod,
						Value: stringValue("POST"),
					},
					{
						Key:   otel.EnvoyMappingNames.NetworkProtocolVersion,
						Value: stringValue("1.1"),
					},
					{
						Key:   otel.EnvoyMappingNames.UrlFull,
						Value: stringValue("https://integration.com/order/123?param1=value1"),
					},
					{
						Key:   otel.EnvoyMappingNames.IntegrationID,
						Value: stringValue(integrationID),
					},
					{
						Key:   "guid:x-request-id",
						Value: stringValue("req-id-value"),
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
	return sendTraces(s.URL, &coltracepb.ExportTraceServiceRequest{
		ResourceSpans: resourceSpans,
	})
}
