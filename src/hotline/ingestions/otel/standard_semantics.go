package otel

import (
	"fmt"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"hotline/ingestions"
	"hotline/integrations"
	"net/url"
	"time"
)

type AttributeNames struct {
	HttpRequestMethod      string // required
	HttpStatusCode         string // conditionally required if no errorType
	UrlFull                string // required
	NetworkProtocolVersion string // Recommended
	IntegrationID          string // Recommended
	ErrorType              string // conditionally required if no status code
	CorrelationID          string // required
}

var StandardMappingNames = AttributeNames{
	HttpRequestMethod:      "http.request.method",
	HttpStatusCode:         "http.response.status_code",
	UrlFull:                "url.full",
	NetworkProtocolVersion: "network.protocol.version",
	IntegrationID:          "user_agent.original",
	ErrorType:              "error.type",
}

type StandardMapping struct {
	attNames AttributeNames
}

func NewStandardMapping() *StandardMapping {
	return &StandardMapping{attNames: StandardMappingNames}
}

// https://opentelemetry.io/docs/specs/semconv/http/http-spans/#http-client
func (h *StandardMapping) ConvertMessageToHttp(reqProto *coltracepb.ExportTraceServiceRequest) []*ingestions.HttpRequest {
	var requests []*ingestions.HttpRequest
	for _, resource := range reqProto.ResourceSpans {
		for _, scope := range resource.ScopeSpans {
			for _, span := range scope.Spans {
				if span.Kind != tracepb.Span_SPAN_KIND_CLIENT {
					continue
				}
				attrs := toMap(span.Attributes)
				id := fmt.Sprintf("%s:%s", span.TraceId, span.SpanId)
				correlationID, _ := attrs.GetStringValue(h.attNames.CorrelationID)
				method, foundMethod := attrs.GetStringValue(h.attNames.HttpRequestMethod)
				if !foundMethod {
					continue
				}
				statusCode, foundStatusCode := attrs.GetStringValue(h.attNames.HttpStatusCode)
				errorType, foundErrorType := attrs.GetStringValue(h.attNames.ErrorType)
				if !foundStatusCode && !foundErrorType {
					continue
				}

				fullUrlString, foundFullUrl := attrs.GetStringValue(h.attNames.UrlFull)
				if !foundFullUrl {
					continue
				}
				fullUrl, fullUrlParseErr := url.Parse(fullUrlString)
				if fullUrl == nil || fullUrlParseErr != nil {
					continue
				}

				protocolVersion, _ := attrs.GetStringValue(h.attNames.NetworkProtocolVersion)
				integrationID := fullUrl.Host
				hotlineIntegrationId, found := attrs.GetStringValue(h.attNames.IntegrationID)
				if found {
					integrationID = hotlineIntegrationId
				}
				if len(integrationID) == 0 {
					continue
				}

				requests = append(requests, &ingestions.HttpRequest{
					ID:              id,
					IntegrationID:   integrations.ID(integrationID),
					ProtocolVersion: protocolVersion,
					Method:          method,
					StatusCode:      statusCode,
					URL:             fullUrl,
					StartTime:       time.Unix(0, int64(span.StartTimeUnixNano)).UTC(),
					EndTime:         time.Unix(0, int64(span.EndTimeUnixNano)).UTC(),
					ErrorType:       errorType,
					CorrelationID:   correlationID,
				})
			}
		}
	}
	return requests
}

type AttributePBMap map[string]*commonpb.KeyValue

func toMap(attributes []*commonpb.KeyValue) AttributePBMap {
	values := make(AttributePBMap)
	for _, attribute := range attributes {
		values[attribute.Key] = attribute
	}
	return values
}

func (m AttributePBMap) GetStringValue(name string) (string, bool) {
	attr, found := m[name]
	if found {
		return attr.Value.GetStringValue(), true
	}
	return "", false
}
