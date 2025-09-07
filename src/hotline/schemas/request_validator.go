package schemas

import (
	"errors"
	"fmt"
	"io"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

type RequestValidator struct {
	header *jsonschema.Schema
	query  *jsonschema.Schema
	body   *jsonschema.Schema
}

type RequestSchema struct {
	ID      ID
	Headers io.Reader
	Query   io.Reader
	Body    io.Reader
}

func NewRequestValidator(definitions RequestSchema) (*RequestValidator, error) {
	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft2020)
	c.UseLoader(&nopLoader{})
	validator := &RequestValidator{}
	if definitions.Headers != nil {
		url := fmt.Sprintf("https://local-server/config-api/request-schemas/%s/files/request-headers.json", definitions.ID)
		headerSchema, err := parse(c, url, definitions.Headers)
		if err != nil {
			return nil, err
		}
		validator.header = headerSchema
	}

	if definitions.Query != nil {
		url := fmt.Sprintf("https://local-server/config-api/request-schemas/%s/files/request-query.json", definitions.ID)
		querySchema, err := parse(c, url, definitions.Query)
		if err != nil {
			return nil, err
		}
		validator.query = querySchema
	}

	if definitions.Body != nil {
		url := fmt.Sprintf("https://local-server/config-api/request-schemas/%s/files/request-body.json", definitions.ID)
		bodySchema, err := parse(c, url, definitions.Body)
		if err != nil {
			return nil, err
		}
		validator.body = bodySchema
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
		validationErr := v.header.Validate(castToAny(headers))
		if validationErr != nil {
			return validationErr
		}
	}
	return nil
}

func castToAny(headers map[string][]string) map[string]any {
	headersAny := make(map[string]any, len(headers))
	for mapKey, mapVal := range headers {
		valSlice := make([]any, len(mapVal))
		for index, valArray := range mapVal {
			valSlice[index] = valArray
		}
		headersAny[mapKey] = valSlice
	}
	return headersAny
}

func (v *RequestValidator) ValidateQuery(query map[string][]string) error {
	if v.query != nil {
		validationErr := v.query.Validate(castToAny(query))
		if validationErr != nil {
			return validationErr
		}
	}
	return nil
}

func (v *RequestValidator) ValidateBody(bodyReader io.Reader) error {
	if v.body != nil {
		bodyMap, readErr := jsonschema.UnmarshalJSON(bodyReader)
		if readErr != nil {
			return readErr
		}

		validationErr := v.body.Validate(bodyMap)
		if validationErr != nil {
			return validationErr
		}
	}
	return nil
}

type nopLoader struct {
}

func (l *nopLoader) Load(_ string) (any, error) {
	return nil, errors.New("do not support loading schemas from remote sources")
}
