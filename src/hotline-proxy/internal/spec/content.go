package spec

import (
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
)

// insideJSON is the location token between a string that holds JSON and a
// place inside that JSON. It can't be a JSON property name the proxy sees,
// since the JSON reader never gives a name holding a zero byte from text
// that doesn't write one.
const insideJSON = "\x00json"

// jsonText checks a string marked contentMediaType: application/json with
// a contentSchema, such as an SQS message in MessageBody: the string must
// be JSON, and that JSON must match the contentSchema. The JSON Schema
// library can do this itself, but reports the wrong place for the string,
// so the proxy does it here.
func jsonText() *jsonschema.Vocabulary {
	return &jsonschema.Vocabulary{
		URL: "https://spec.hotline.invalid/vocab/json-text",
		Compile: func(ctx *jsonschema.CompilerContext, obj map[string]any) (jsonschema.SchemaExt, error) {
			mediaType, _ := obj["contentMediaType"].(string)
			if _, ok := obj["contentSchema"]; !ok || !strings.EqualFold(mediaType, "application/json") {
				return nil, nil //nolint:nilnil // no such keywords here, as the library asks
			}
			return &jsonTextSchema{inner: ctx.Enqueue([]string{"contentSchema"})}, nil
		},
	}
}

type jsonTextSchema struct {
	inner *jsonschema.Schema
}

func (s *jsonTextSchema) Validate(ctx *jsonschema.ValidatorContext, v any) {
	text, isText := v.(string)
	decoded, err := jsonschema.UnmarshalJSON(strings.NewReader(text))
	if !isText || err != nil {
		ctx.AddError(&kind.ContentMediaType{Want: "application/json"})
		return
	}
	if err := ctx.Validate(s.inner, decoded, []string{insideJSON}); err != nil {
		ctx.AddErr(err)
	}
}
