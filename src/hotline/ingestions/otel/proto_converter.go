package otel

import (
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"hotline/ingestions"
)

type ProtoConverter struct {
	standard *StandardMapping
	envoy    *EnvoyMapping
}

func NewProtoConverter() *ProtoConverter {
	return &ProtoConverter{
		standard: NewStandardMapping(),
		envoy:    NewEnvoyMapping(),
	}
}

func (p *ProtoConverter) Convert(c *coltracepb.ExportTraceServiceRequest) []*ingestions.HttpRequest {

	reqs := p.envoy.ConvertMessageToHttp(c)
	if len(reqs) > 0 {
		return reqs
	}

	return p.standard.ConvertMessageToHttp(c)
}
