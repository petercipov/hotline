package otel

// TracesMessage format found here https://github.com/open-telemetry/opentelemetry-proto/blob/main/examples/trace.json
// https://github.com/open-telemetry/opentelemetry-proto/blob/main/opentelemetry/proto/trace/v1/trace.proto
type TracesMessage struct {
	ResourceSpans []ResourceSpan `json:"resourceSpans"`
}

type ResourceSpan struct {
	Resource   Resource    `json:"resource"`
	ScopeSpans []ScopeSpan `json:"scopeSpans"`
}

type ScopeSpan struct {
	Scope Scope  `json:"scope"`
	Spans []Span `json:"spans"`
}

type Resource struct {
	Attributes AttributeList `json:"attributes"`
}

type Scope struct {
	Name       string        `json:"name"`
	Version    string        `json:"version"`
	Attributes AttributeList `json:"attributes"`
}

type Span struct {
	TraceId           string        `json:"traceId"`
	SpanId            string        `json:"spanId"`
	ParentSpanId      string        `json:"parentSpanId"`
	Name              string        `json:"name"`
	StartTimeUnixNano string        `json:"startTimeUnixNano"`
	EndTimeUnixNano   string        `json:"endTimeUnixNano"`
	Kind              int           `json:"kind"`
	Attributes        AttributeList `json:"attributes"`
}

type AttributeList []Attribute
type AttributeMap map[string]Attribute

func (l AttributeList) ToMap() AttributeMap {
	attrs := make(AttributeMap, len(l))
	for _, attr := range l {
		attrs[attr.Key] = attr
	}
	return attrs
}

func (l AttributeList) Remove(name string) AttributeList {
	returnList := l
	for i, attr := range l {
		if attr.Key == name {
			l[i] = l[len(l)-1]
			returnList = l[:len(l)-1]
			break
		}
	}
	return returnList
}

func (m AttributeMap) GetStringValue(name string) (string, bool) {
	attr, found := m[name]
	if found {
		return attr.Value["stringValue"].(string), true
	}
	return "", false
}

type Attribute struct {
	Key   string                 `json:"key"`
	Value map[string]interface{} `json:"value"`
}
