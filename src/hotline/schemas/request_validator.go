package schemas

import (
	"fmt"
	"io"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

type RequestValidator struct {
	header *jsonschema.Schema
}

type RequestSchema struct {
	ID      ID
	Headers io.Reader
}

func NewRequestValidator(definitions RequestSchema) (*RequestValidator, error) {
	c := jsonschema.NewCompiler()
	validator := &RequestValidator{}
	if definitions.Headers != nil {
		url := fmt.Sprintf("https://local-server/config-api/request-schemas/%s/files/request-headers.json", definitions.ID)
		headerSchema, err := parse(c, url, definitions.Headers)
		if err != nil {
			return nil, err
		}
		validator.header = headerSchema
	}

	return validator, nil
}

func parse(c *jsonschema.Compiler, url string, r io.Reader) (*jsonschema.Schema, error) {
	schema, readErr := jsonschema.UnmarshalJSON(r)
	if readErr != nil {
		return nil, readErr
	}

	_ = c.AddResource(url, schema)
	compiledSchema, compileErr := c.Compile(url)
	if compileErr != nil {
		return nil, compileErr
	}
	return compiledSchema, nil
}

func (v *RequestValidator) ValidateHeaders(headers map[string][]string) error {
	if v.header != nil {
		headersAny := make(map[string]any, len(headers))
		for mapKey, mapVal := range headers {
			valSlice := make([]any, len(mapVal))
			for index, valArray := range mapVal {
				valSlice[index] = valArray
			}
			headersAny[mapKey] = valSlice
		}
		validationErr := v.header.Validate(headersAny)
		if validationErr != nil {
			return validationErr
		}
	}
	return nil
}
