package schemas

import (
	"errors"
	"fmt"
	"io"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

var ErrNoContent = errors.New("no content provided")

type Validator struct {
	requestHeaders *jsonSchemaValidator
	requestQuery   *jsonSchemaValidator
	requestBody    *jsonSchemaValidator

	responseHeaders *jsonSchemaValidator
	responseBody    *jsonSchemaValidator
}

type RequestSchema struct {
	RequestHeaders *SchemaDefinition
	RequestQuery   *SchemaDefinition
	RequestBody    *SchemaDefinition

	ResponseHeaders *SchemaDefinition
	ResponseBody    *SchemaDefinition
}

type SchemaDefinition struct {
	ID      ID
	Content io.Reader
}

type jsonSchemaValidator struct {
	schemaID ID
	schema   *jsonschema.Schema
}

func (j *jsonSchemaValidator) ValidateMap(headers map[string][]string) (*ID, *ValidationError) {
	if j == nil {
		return nil, nil
	}
	validationErr := j.schema.Validate(castToAny(headers))
	if validationErr != nil {
		return &j.schemaID, &ValidationError{
			SchemaID: j.schemaID,
			Err:      validationErr,
		}
	}
	return &j.schemaID, nil
}

func (j *jsonSchemaValidator) ValidateInput(content io.Reader) (*ID, *ValidationError) {
	if j == nil {
		return nil, nil
	}

	if content == nil {
		return &j.schemaID, &ValidationError{
			SchemaID: j.schemaID,
			Err:      ErrNoContent,
		}
	}

	contentMap, readErr := jsonschema.UnmarshalJSON(content)
	if readErr != nil {
		return &j.schemaID, &ValidationError{
			SchemaID: j.schemaID,
			Err:      readErr,
		}
	}

	validationErr := j.schema.Validate(contentMap)
	if validationErr != nil {
		return &j.schemaID, &ValidationError{
			SchemaID: j.schemaID,
			Err:      validationErr,
		}
	}
	return &j.schemaID, nil
}

func newJsonSchemaValidator(definition SchemaDefinition, name string, c *jsonschema.Compiler) (*jsonSchemaValidator, error) {
	url := fmt.Sprintf("https://local-server/api/v1/request-schemas/%s/files/%s.json", definition.ID, name)
	schema, err := parse(c, url, definition.Content)
	if err != nil {
		return nil, err
	}
	return &jsonSchemaValidator{
		schemaID: definition.ID,
		schema:   schema,
	}, nil
}

func NewRequestValidator(definitions RequestSchema) (*Validator, error) {
	c := createCompiler()
	validator := &Validator{}

	var parseErr error
	if definitions.RequestHeaders != nil {
		validator.requestHeaders, parseErr = newJsonSchemaValidator(*definitions.RequestHeaders, "request-headers", c)
		if parseErr != nil {
			return nil, parseErr
		}
	}

	if definitions.RequestQuery != nil {
		validator.requestQuery, parseErr = newJsonSchemaValidator(*definitions.RequestQuery, "request-query", c)
		if parseErr != nil {
			return nil, parseErr
		}
	}

	if definitions.RequestBody != nil {
		validator.requestBody, parseErr = newJsonSchemaValidator(*definitions.RequestBody, "request-body", c)
		if parseErr != nil {
			return nil, parseErr
		}
	}

	if definitions.ResponseHeaders != nil {
		validator.responseHeaders, parseErr = newJsonSchemaValidator(*definitions.ResponseHeaders, "response-headers", c)
		if parseErr != nil {
			return nil, parseErr
		}
	}

	if definitions.ResponseBody != nil {
		validator.responseBody, parseErr = newJsonSchemaValidator(*definitions.ResponseBody, "response-body", c)
		if parseErr != nil {
			return nil, parseErr
		}
	}
	return validator, nil
}

func createCompiler() *jsonschema.Compiler {
	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft2020)
	c.UseLoader(&nopLoader{})
	return c
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

func (v *Validator) ValidateHeaders(headers map[string][]string) (*ID, *ValidationError) {
	return v.requestHeaders.ValidateMap(headers)
}

func (v *Validator) ValidateQuery(query map[string][]string) (*ID, *ValidationError) {
	return v.requestQuery.ValidateMap(query)
}

func (v *Validator) ValidateBody(bodyReader io.Reader) (*ID, *ValidationError) {
	return v.requestBody.ValidateInput(bodyReader)
}

func (v *Validator) ValidateResponseHeaders(headers map[string][]string) (*ID, *ValidationError) {
	return v.responseHeaders.ValidateMap(headers)
}

func (v *Validator) ValidateResponseBody(bodyReader io.Reader) (*ID, *ValidationError) {
	return v.responseBody.ValidateInput(bodyReader)
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

type nopLoader struct {
}

var ErrRemoteSchemaNotSupported = errors.New("do not support loading schemas from remote sources")

func (l *nopLoader) Load(_ string) (any, error) {
	return nil, ErrRemoteSchemaNotSupported
}
