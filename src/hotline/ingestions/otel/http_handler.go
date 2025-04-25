package otel

import (
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/proto"
	"hotline/ingestions"
	"io"
	"net/http"
)

type TracesHandler struct {
	ingestion           func([]*ingestions.HttpRequest)
	convertProtoMessage func(c *coltracepb.ExportTraceServiceRequest) []*ingestions.HttpRequest
}

func NewTracesHandler(ingestion func([]*ingestions.HttpRequest), convertProtoMessage func(c *coltracepb.ExportTraceServiceRequest) []*ingestions.HttpRequest) *TracesHandler {
	return &TracesHandler{
		ingestion:           ingestion,
		convertProtoMessage: convertProtoMessage,
	}
}

func (h *TracesHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	raw, readErr := io.ReadAll(req.Body)
	defer req.Body.Close()
	if readErr != nil {
		http.Error(w, "could not read body", http.StatusInternalServerError)
		return
	}

	var reqProto coltracepb.ExportTraceServiceRequest
	unmarshalErr := proto.Unmarshal(raw, &reqProto)
	if unmarshalErr != nil {
		http.Error(w, "could not parse proto", http.StatusBadRequest)
		return
	}
	h.ingestion(h.convertProtoMessage(&reqProto))
	w.WriteHeader(http.StatusCreated)
}
