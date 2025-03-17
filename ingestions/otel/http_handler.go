package otel

import (
	"encoding/json"
	"fmt"
	"hotline/ingestions"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
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
}

var DefaultAttributeNames = AttributeNames{
	HttpRequestMethod:      "http.request.method",
	HttpStatusCode:         "http.response.status_code",
	UrlFull:                "url.full",
	NetworkProtocolVersion: "network.protocol.version",
	IntegrationID:          "integration.id",
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

	var message TracesMessage
	unmarshalErr := json.Unmarshal(raw, &message)
	if unmarshalErr != nil {
		http.Error(w, "could not parse json", http.StatusBadRequest)
	}

	h.ingestion.Ingest(h.convertMessageToHttp(message))
	w.WriteHeader(http.StatusCreated)
}

func (h *TracesHandler) convertMessageToHttp(message TracesMessage) []ingestions.HttpRequest {
	var requests []ingestions.HttpRequest
	for _, resource := range message.ResourceSpans {
		for _, scope := range resource.ScopeSpans {
			for _, span := range scope.Spans {
				attrs := span.Attributes.ToMap()
				id := fmt.Sprintf("%s:%s", span.TraceId, span.SpanId)
				method, _ := attrs.GetValue(h.attNames.HttpRequestMethod)
				statusCode, _ := attrs.GetValue(h.attNames.HttpStatusCode)

				fullUrlString, _ := attrs.GetValue(h.attNames.UrlFull)
				fullUrl, _ := url.Parse(fullUrlString)

				protocolVersion, _ := attrs.GetValue(h.attNames.NetworkProtocolVersion)

				integrationID := fullUrl.Host
				hotlineIntegrationId, found := attrs.GetValue(h.attNames.IntegrationID)
				if found {
					integrationID = hotlineIntegrationId
				}

				startTimeNano, _ := strconv.ParseInt(span.StartTimeUnixNano, 10, 64)
				endTimeNano, _ := strconv.ParseInt(span.EndTimeUnixNano, 10, 64)

				requests = append(requests, ingestions.HttpRequest{
					ID:              id,
					IntegrationID:   integrationID,
					ProtocolVersion: protocolVersion,
					Method:          method,
					StatusCode:      statusCode,
					URL:             fullUrl,
					StartTime:       time.Unix(0, startTimeNano).UTC(),
					EndTime:         time.Unix(0, endTimeNano).UTC(),
				})
			}
		}
	}
	return requests
}
