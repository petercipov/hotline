package setup_test

import (
	"bytes"
	"context"
	"hotline/clock"
	"hotline/ingestions/otel"
	"math/rand"
	"net/http"
	"strconv"
	"time"

	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

type OTELClient struct {
	URL string
}

func (s *OTELClient) SendSomeTraffic(now time.Time, integrationID string) (int, error) {
	r := rand.New(rand.NewSource(0))

	statusCodes := []string{
		"200", "201", "403", "500", "503",
	}

	names := otel.StandardMappingNames()

	return s.sendTraffic(integrationID, 10, 1000, func(span *tracepb.Span) {

		startTime := now
		endTime := startTime.Add(time.Duration(r.Intn(10000)) * time.Millisecond)

		span.EndTimeUnixNano = clock.TimeToUint64NanoOrZero(endTime)
		span.StartTimeUnixNano = clock.TimeToUint64NanoOrZero(startTime)

		statusCode := statusCodes[r.Intn(len(statusCodes))]
		span.Attributes = append(span.Attributes, &commonpb.KeyValue{
			Key:   names.HttpStatusCode,
			Value: stringValue(statusCode),
		})
	})
}

func (s *OTELClient) sendTraffic(integrationID string, resourceCount int, traceCount int, modifier func(span *tracepb.Span)) (int, error) {
	var resourceSpans []*tracepb.ResourceSpans
	names := otel.StandardMappingNames()
	for ri := range resourceCount {
		var spans []*tracepb.Span
		for ti := range traceCount {
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
						Key:   names.HttpRequestMethod,
						Value: stringValue("POST"),
					},
					{
						Key:   names.NetworkProtocolVersion,
						Value: stringValue("1.1"),
					},
					{
						Key:   names.UrlFull,
						Value: stringValue("https://integration.com/order/123?param1=value1"),
					},
					{
						Key:   names.IntegrationID,
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
	return sendTraces(s.URL, &coltracepb.ExportTraceServiceRequest{
		ResourceSpans: resourceSpans,
	})
}

func sendTraces(url string, message *coltracepb.ExportTraceServiceRequest) (int, error) {
	raw, marshalErr := proto.Marshal(message)
	if marshalErr != nil {
		return 0, marshalErr
	}
	// https://github.com/open-telemetry/opentelemetry-collector/blob/432d92d8b366f6831323a928783f1ed867c42050/exporter/otlphttpexporter/otlp.go#L185
	req, createErr := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewReader(raw))
	if createErr != nil {
		return 0, createErr
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "remote-service-otel-exporter")
	resp, reqErr := http.DefaultClient.Do(req)
	if reqErr != nil {
		return 0, reqErr
	}
	_ = resp.Body.Close()
	return resp.StatusCode, nil
}

func stringValue(value string) *commonpb.AnyValue {
	return &commonpb.AnyValue{
		Value: &commonpb.AnyValue_StringValue{StringValue: value},
	}
}
