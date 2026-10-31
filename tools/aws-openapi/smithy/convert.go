// Package smithy turns an AWS service's Smithy model (AWS's own api
// description language, in its JSON form) into an OpenAPI 3.1 spec Hotline
// can read. Only services on AWS's JSON protocol are handled: every call is
// POST /, and the action is named in the X-Amz-Target header.
package smithy

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// ErrUnsupported marks a model this converter can't turn into a spec.
var ErrUnsupported = errors.New("smithy: unsupported model")

const actionHeader = "X-Amz-Target"

type model struct {
	Shapes map[string]shape `json:"shapes"`
}

type shape struct {
	Type       string            `json:"type"`
	Version    string            `json:"version"`
	Operations []ref             `json:"operations"`
	Input      *ref              `json:"input"`
	Output     *ref              `json:"output"`
	Members    map[string]member `json:"members"`
	Member     *member           `json:"member"`
	Key        *member           `json:"key"`
	Value      *member           `json:"value"`
	Traits     map[string]any    `json:"traits"`
}

type ref struct {
	Target string `json:"target"`
}

type member struct {
	Target string         `json:"target"`
	Traits map[string]any `json:"traits"`
}

// document is the spec's top level, in the order it is written.
type document struct {
	OpenAPI      string         `yaml:"openapi"`
	Info         map[string]any `yaml:"info"`
	ActionHeader string         `yaml:"x-hotline-action-header"` //nolint:tagliatelle // the name of our OpenAPI extension
	Servers      []any          `yaml:"servers"`
	Security     []any          `yaml:"security"`
	Paths        map[string]any `yaml:"paths"`
	Components   map[string]any `yaml:"components"`
}

// Convert reads a Smithy model holding one AWS JSON service and writes its
// OpenAPI spec as YAML. record maps a field of an action's input, such as
// QueueUrl, to the record attribute the proxy copies it to, such as
// messaging.destination.name; the field is marked x-hotline-record.
func Convert(data []byte, record map[string]string) ([]byte, error) {
	var m model
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("%w: not a Smithy JSON model: %w", ErrUnsupported, err)
	}
	serviceID, service, err := m.service()
	if err != nil {
		return nil, err
	}
	var mediaType string
	switch {
	case has(service.Traits, "aws.protocols#awsJson1_0"):
		mediaType = "application/x-amz-json-1.0"
	case has(service.Traits, "aws.protocols#awsJson1_1"):
		mediaType = "application/x-amz-json-1.1"
	default:
		return nil, fmt.Errorf("%w: %s is not on AWS's JSON protocol", ErrUnsupported, serviceID)
	}
	c := converter{model: m, mediaType: mediaType, schemas: map[string]any{}, record: record}
	prefix := name(serviceID)

	paths := map[string]any{}
	for _, op := range service.Operations {
		paths["/#"+actionHeader+"="+prefix+"."+name(op.Target)] = map[string]any{"post": c.operation(op.Target)}
	}
	c.schemas["AwsJsonError"] = map[string]any{
		"type":        "object",
		"description": "The body of every AWS JSON error reply: __type names the error.",
		"required":    []string{"__type"},
		"properties": map[string]any{
			"__type":  map[string]any{"type": "string"},
			"message": map[string]any{"type": "string"},
			"Message": map[string]any{"type": "string"},
		},
	}

	title := name(serviceID)
	if t, ok := service.Traits["smithy.api#title"].(string); ok {
		title = t
	}
	doc := document{
		OpenAPI: "3.1.0",
		Info: map[string]any{
			"title":       title,
			"version":     service.Version,
			"description": "Made by Hotline's aws-openapi tool from AWS's Smithy model. Do not edit by hand.",
		},
		ActionHeader: actionHeader,
		Servers:      []any{map[string]any{"url": "https://" + endpointPrefix(service, title) + ".{region}.amazonaws.com", "variables": map[string]any{"region": map[string]any{"default": "us-east-1"}}}},
		Security:     []any{map[string]any{"aws_sigv4": []any{}}},
		Paths:        paths,
		Components: map[string]any{
			"securitySchemes": map[string]any{
				// AWS's signature is a header the proxy never reads.
				"aws_sigv4": map[string]any{"type": "apiKey", "in": "header", "name": "Authorization"},
			},
			"schemas": c.schemas,
		},
	}
	return yaml.Marshal(doc)
}

func (m model) service() (string, shape, error) {
	var ids []string
	for id, s := range m.Shapes {
		if s.Type == "service" {
			ids = append(ids, id)
		}
	}
	if len(ids) != 1 {
		return "", shape{}, fmt.Errorf("%w: the model must hold exactly one service, it holds %d", ErrUnsupported, len(ids))
	}
	return ids[0], m.Shapes[ids[0]], nil
}

func endpointPrefix(service shape, fallback string) string {
	if s, ok := service.Traits["aws.api#service"].(map[string]any); ok {
		if p, ok := s["endpointPrefix"].(string); ok {
			return p
		}
	}
	return strings.ToLower(fallback)
}

type converter struct {
	model     model
	mediaType string
	schemas   map[string]any
	record    map[string]string
}

func (c *converter) operation(id string) map[string]any {
	op := c.model.Shapes[id]
	requestID := map[string]any{"x-amzn-RequestId": map[string]any{"schema": map[string]any{"type": "string"}, "x-hotline-redact": false}}
	errorHeaders := map[string]any{
		"x-amzn-RequestId":   map[string]any{"schema": map[string]any{"type": "string"}, "x-hotline-redact": false},
		"x-amzn-query-error": map[string]any{"schema": map[string]any{"type": "string"}, "x-hotline-redact": false},
	}
	errorReply := map[string]any{
		"description": "An AWS error.",
		"headers":     errorHeaders,
		"content":     c.content(map[string]any{"$ref": "#/components/schemas/AwsJsonError"}),
	}
	input := c.body(op.Input)
	c.mark(op.Input)
	out := map[string]any{
		"operationId": name(id),
		"requestBody": map[string]any{"required": true, "content": c.content(input)},
		"responses": map[string]any{
			"200": map[string]any{"description": "Done.", "headers": requestID, "content": c.content(c.body(op.Output))},
			"4XX": errorReply,
			"5XX": errorReply,
		},
	}
	if doc := text(op.Traits); doc != "" {
		out["description"] = doc
	}
	return out
}

// mark marks the fields of an action's input that are copied into records.
// Only inputs are marked, so an output that shares a field's name is not.
func (c *converter) mark(r *ref) {
	if r == nil || r.Target == "smithy.api#Unit" {
		return
	}
	schema, _ := c.schemas[name(r.Target)].(map[string]any)
	props, _ := schema["properties"].(map[string]any)
	for field, attribute := range c.record {
		if prop, ok := props[field].(map[string]any); ok {
			prop["x-hotline-record"] = attribute
		}
	}
}

func (c *converter) content(schema any) map[string]any {
	// The proxy reads AWS's own name for JSON as JSON only when marked.
	return map[string]any{c.mediaType: map[string]any{"schema": schema, "x-hotline-body": "json"}}
}

// body is the schema of an action's input or output. An action without
// one still sends or gets an empty JSON object.
func (c *converter) body(r *ref) any {
	if r == nil || r.Target == "smithy.api#Unit" {
		return map[string]any{"type": "object"}
	}
	return c.schemaRef(r.Target)
}

// schemaRef points to a named shape, writing it into the components the
// first time. Smithy's own simple shapes are written in place.
func (c *converter) schemaRef(target string) map[string]any {
	if strings.HasPrefix(target, "smithy.api#") {
		return prelude(target)
	}
	n := name(target)
	if _, done := c.schemas[n]; !done {
		c.schemas[n] = nil // marks it as started, for shapes that refer to themselves
		c.schemas[n] = c.shapeSchema(c.model.Shapes[target])
	}
	return map[string]any{"$ref": "#/components/schemas/" + n}
}

func (c *converter) shapeSchema(s shape) map[string]any {
	var out map[string]any
	switch s.Type {
	case "structure":
		props := map[string]any{}
		var required []string
		for _, n := range sortedKeys(s.Members) {
			mb := s.Members[n]
			props[jsonName(n, mb)] = c.memberSchema(mb)
			if has(mb.Traits, "smithy.api#required") {
				required = append(required, jsonName(n, mb))
			}
		}
		out = map[string]any{"type": "object", "properties": props}
		if len(required) > 0 {
			out["required"] = required
		}
	case "union":
		props := map[string]any{}
		for _, n := range sortedKeys(s.Members) {
			props[jsonName(n, s.Members[n])] = c.memberSchema(s.Members[n])
		}
		out = map[string]any{"type": "object", "properties": props, "minProperties": 1, "maxProperties": 1}
	case "list", "set":
		out = map[string]any{"type": "array", "items": c.memberSchema(*s.Member)}
	case "map":
		out = map[string]any{"type": "object", "additionalProperties": c.memberSchema(*s.Value)}
	case "enum":
		var values []string
		for _, n := range sortedKeys(s.Members) {
			v, ok := s.Members[n].Traits["smithy.api#enumValue"].(string)
			if !ok {
				v = n
			}
			values = append(values, v)
		}
		out = map[string]any{"type": "string", "enum": values}
	case "intEnum":
		var values []any
		for _, n := range sortedKeys(s.Members) {
			values = append(values, s.Members[n].Traits["smithy.api#enumValue"])
		}
		out = map[string]any{"type": "integer", "enum": values}
	default:
		out = simple(s.Type)
	}
	constrain(out, s.Traits, s.Type)
	if doc := text(s.Traits); doc != "" && s.Type == "structure" {
		out["description"] = doc
	}
	return out
}

func (c *converter) memberSchema(mb member) map[string]any {
	base := c.schemaRef(mb.Target)
	out := map[string]any{}
	for k, v := range base {
		out[k] = v
	}
	kind := ""
	if strings.HasPrefix(mb.Target, "smithy.api#") {
		kind = strings.ToLower(strings.TrimPrefix(mb.Target, "smithy.api#"))
	} else {
		kind = c.model.Shapes[mb.Target].Type
	}
	constrain(out, mb.Traits, kind)
	if doc := text(mb.Traits); doc != "" {
		out["description"] = doc
	}
	return out
}

// constrain adds Smithy's length, range and pattern rules.
func constrain(out map[string]any, traits map[string]any, kind string) {
	if l, ok := traits["smithy.api#length"].(map[string]any); ok {
		minKey, maxKey := "minLength", "maxLength"
		switch kind {
		case "list", "set":
			minKey, maxKey = "minItems", "maxItems"
		case "map", "structure":
			minKey, maxKey = "minProperties", "maxProperties"
		}
		if v, ok := l["min"]; ok {
			out[minKey] = v
		}
		if v, ok := l["max"]; ok {
			out[maxKey] = v
		}
	}
	if r, ok := traits["smithy.api#range"].(map[string]any); ok {
		if v, ok := r["min"]; ok {
			out["minimum"] = v
		}
		if v, ok := r["max"]; ok {
			out["maximum"] = v
		}
	}
	if p, ok := traits["smithy.api#pattern"].(string); ok {
		out["pattern"] = p
	}
}

func prelude(target string) map[string]any {
	t := strings.ToLower(strings.TrimPrefix(target, "smithy.api#"))
	t = strings.TrimPrefix(t, "primitive")
	return simple(t)
}

// simple is the JSON Schema for one of Smithy's simple shape types, as AWS's
// JSON protocol writes them.
func simple(kind string) map[string]any {
	switch kind {
	case "string":
		return map[string]any{"type": "string"}
	case "boolean":
		return map[string]any{"type": "boolean"}
	case "byte", "short", "integer":
		return map[string]any{"type": "integer", "format": "int32"}
	case "long":
		return map[string]any{"type": "integer", "format": "int64"}
	case "biginteger":
		return map[string]any{"type": "integer"}
	case "float", "double", "bigdecimal":
		return map[string]any{"type": "number"}
	case "timestamp":
		// AWS JSON writes times as seconds since 1970, with a fraction.
		return map[string]any{"type": "number"}
	case "blob":
		return map[string]any{"type": "string", "contentEncoding": "base64"}
	default: // document, and anything new: any JSON value
		return map[string]any{}
	}
}

func jsonName(n string, mb member) string {
	if j, ok := mb.Traits["smithy.api#jsonName"].(string); ok {
		return j
	}
	return n
}

func has(traits map[string]any, trait string) bool {
	_, ok := traits[trait]
	return ok
}

// name is the part of a shape ID after the #.
func name(id string) string {
	_, n, _ := strings.Cut(id, "#")
	return n
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

var (
	tags   = regexp.MustCompile(`<[^>]*>`)
	spaces = regexp.MustCompile(`\s+`)
)

// text is a shape's documentation as plain text: AWS writes it as HTML.
func text(traits map[string]any) string {
	doc, ok := traits["smithy.api#documentation"].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(spaces.ReplaceAllString(html.UnescapeString(tags.ReplaceAllString(doc, " ")), " "))
}
