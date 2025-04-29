package otel

import (
	"fmt"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"hotline/ingestions"
	"hotline/integrations"
	"net/url"
	"time"
)

type EnvoyAttributeNames struct {
	HttpRequestMethod      string // required
	HttpStatusCode         string // conditionally required if no errorType
	UrlFull                string // required
	NetworkProtocolVersion string // Recommended
	IntegrationID          string // Recommended
	CorrelationID          string // optional
}

var EnvoyMappingNames = EnvoyAttributeNames{
	HttpRequestMethod:      "http.method",
	HttpStatusCode:         "http.status_code",
	UrlFull:                "http.url",
	NetworkProtocolVersion: "http.protocol",
	IntegrationID:          "user_agent",
	CorrelationID:          "guid:x-request-id",
}

type EnvoyMapping struct {
	attNames EnvoyAttributeNames
}

func NewEnvoyMapping() *EnvoyMapping {
	return &EnvoyMapping{attNames: EnvoyMappingNames}
}

func (h *EnvoyMapping) ConvertMessageToHttp(reqProto *coltracepb.ExportTraceServiceRequest) []*ingestions.HttpRequest {
	var requests []*ingestions.HttpRequest
	for _, resource := range reqProto.ResourceSpans {
		for _, scope := range resource.ScopeSpans {
			for _, span := range scope.Spans {
				attrs := toMap(span.Attributes)
				id := fmt.Sprintf("%s:%s", span.TraceId, span.SpanId)
				correlationID, _ := attrs.GetStringValue(h.attNames.CorrelationID)
				method, foundMethod := attrs.GetStringValue(h.attNames.HttpRequestMethod)
				if !foundMethod {
					continue
				}
				statusCode, foundStatusCode := attrs.GetStringValue(h.attNames.HttpStatusCode)
				if !foundStatusCode {
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

				requests = append(requests, &ingestions.HttpRequest{
					ID:              id,
					IntegrationID:   integrations.ID(integrationID),
					ProtocolVersion: protocolVersion,
					Method:          method,
					StatusCode:      statusCode,
					URL:             fullUrl,
					StartTime:       time.Unix(0, int64(span.StartTimeUnixNano)).UTC(),
					EndTime:         time.Unix(0, int64(span.EndTimeUnixNano)).UTC(),
					CorrelationID:   correlationID,
				})
			}
		}
	}
	return requests
}
