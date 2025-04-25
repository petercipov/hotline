package setup_test

import (
	"bytes"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
	"hotline/ingestions/otel"
	"math/rand"
	"net/http"
	"strconv"
	"time"
)

type IngestionClient struct {
	URL string
}

func (s *IngestionClient) SendSomeTraffic(now time.Time, integrationID string) (int, error) {
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
			Key:   otel.StandardMappingNames.HttpStatusCode,
			Value: stringValue(statusCode),
		})
	})
}

func (s *IngestionClient) sendTraffic(integrationID string, resourceCount int, traceCount int, modifier func(span *tracepb.Span)) (int, error) {
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
				Kind:              3,
				// https://opentelemetry.io/docs/specs/semconv/http/http-spans/
				Attributes: []*commonpb.KeyValue{
					{
						Key:   otel.StandardMappingNames.HttpRequestMethod,
						Value: stringValue("POST"),
					},
					{
						Key:   otel.StandardMappingNames.NetworkProtocolVersion,
						Value: stringValue("1.1"),
					},
					{
						Key:   otel.StandardMappingNames.UrlFull,
						Value: stringValue("https://integration.com/order/123?param1=value1"),
					},
					{
						Key:   otel.StandardMappingNames.IntegrationID,
						Value: stringValue(integrationID),
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
	return s.sendTraces(&coltracepb.ExportTraceServiceRequest{
		ResourceSpans: resourceSpans,
	})
}

func (s *IngestionClient) sendTraces(message *coltracepb.ExportTraceServiceRequest) (int, error) {
	raw, marshalErr := proto.Marshal(message)
	if marshalErr != nil {
		return 0, marshalErr
	}
	// https://github.com/open-telemetry/opentelemetry-collector/blob/432d92d8b366f6831323a928783f1ed867c42050/exporter/otlphttpexporter/otlp.go#L185
	req, createErr := http.NewRequest(http.MethodPost, s.URL, bytes.NewReader(raw))
	if createErr != nil {
		return 0, createErr
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "remote-service-otel-exporter")
	resp, reqErr := http.DefaultClient.Do(req)
	if reqErr != nil {
		return 0, reqErr
	}
	return resp.StatusCode, nil
}

func stringValue(value string) *commonpb.AnyValue {
	return &commonpb.AnyValue{
		Value: &commonpb.AnyValue_StringValue{StringValue: value},
	}
}
