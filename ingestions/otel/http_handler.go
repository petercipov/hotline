package otel

import (
	"fmt"
	"google.golang.org/protobuf/proto"
	"hotline/ingestions"
	"io"
	"net/http"
	"net/url"
	"time"

	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

type Ingestion interface {
	Ingest([]ingestions.HttpRequest)
}

type AttributeNames struct {
	HttpRequestMethod      string
	HttpStatusCode         string
	UrlFull                string
	NetworkProtocolVersion string
	IntegrationID          string
	ErrorType              string
}

var DefaultAttributeNames = AttributeNames{
	HttpRequestMethod:      "http.request.method",
	HttpStatusCode:         "http.response.status_code",
	UrlFull:                "url.full",
	NetworkProtocolVersion: "network.protocol.version",
	IntegrationID:          "integration.id",
	ErrorType:              "error.type",
}

type TracesHandler struct {
	ingestion Ingestion
	attNames  AttributeNames
}

func NewTracesHandler(ingestion Ingestion, attNames AttributeNames) *TracesHandler {
	return &TracesHandler{
		ingestion: ingestion,
		attNames:  attNames,
	}
}

func (h *TracesHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	raw, readErr := io.ReadAll(req.Body)
	defer req.Body.Close()
	if readErr != nil {
		http.Error(w, "could not read body", http.StatusInternalServerError)
	}

	var reqProto coltracepb.ExportTraceServiceRequest
	unmarshalErr := proto.Unmarshal(raw, &reqProto)
	if unmarshalErr != nil {
		http.Error(w, "could not parse proto", http.StatusBadRequest)
	}
	h.ingestion.Ingest(h.convertMessageToHttp(&reqProto))
	w.WriteHeader(http.StatusCreated)
}

func (h *TracesHandler) convertMessageToHttp(reqProto *coltracepb.ExportTraceServiceRequest) []ingestions.HttpRequest {
	var requests []ingestions.HttpRequest
	for _, resource := range reqProto.ResourceSpans {
		for _, scope := range resource.ScopeSpans {
			for _, span := range scope.Spans {
				if span.Kind != tracepb.Span_SPAN_KIND_CLIENT {
					continue
				}
				attrs := toMap(span.Attributes)
				id := fmt.Sprintf("%s:%s", span.TraceId, span.SpanId)
				method, _ := attrs.GetStringValue(h.attNames.HttpRequestMethod)
				statusCode, _ := attrs.GetStringValue(h.attNames.HttpStatusCode)

				fullUrlString, _ := attrs.GetStringValue(h.attNames.UrlFull)
				fullUrl, _ := url.Parse(fullUrlString)

				protocolVersion, _ := attrs.GetStringValue(h.attNames.NetworkProtocolVersion)

				integrationID := fullUrl.Host
				hotlineIntegrationId, found := attrs.GetStringValue(h.attNames.IntegrationID)
				if found {
					integrationID = hotlineIntegrationId
				}

				errorType, _ := attrs.GetStringValue(h.attNames.ErrorType)

				requests = append(requests, ingestions.HttpRequest{
					ID:              id,
					IntegrationID:   integrationID,
					ProtocolVersion: protocolVersion,
					Method:          method,
					StatusCode:      statusCode,
					URL:             fullUrl,
					StartTime:       time.Unix(0, int64(span.StartTimeUnixNano)).UTC(),
					EndTime:         time.Unix(0, int64(span.EndTimeUnixNano)).UTC(),
					ErrorType:       errorType,
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
