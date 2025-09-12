package schemas

import (
	"errors"
	"fmt"
	"io"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

type Validator struct {
	requestHeaders *jsonschema.Schema
	requestQuery   *jsonschema.Schema
	requestBody    *jsonschema.Schema

	responseHeaders *jsonschema.Schema
	responseBody    *jsonschema.Schema
}

type Schema struct {
	ID             ID
	RequestHeaders io.Reader
	RequestQuery   io.Reader
	RequestBody    io.Reader

	ResponseHeaders io.Reader
	ResponseBody    io.Reader
}

func NewRequestValidator(definitions Schema) (*Validator, error) {
	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft2020)
	c.UseLoader(&nopLoader{})
	validator := &Validator{}
	if definitions.RequestHeaders != nil {
		url := fmt.Sprintf("https://local-server/api/v1/request-schemas/%s/files/request-headers.json", definitions.ID)
		headerSchema, err := parse(c, url, definitions.RequestHeaders)
		if err != nil {
			return nil, err
		}
		validator.requestHeaders = headerSchema
	}

	if definitions.RequestQuery != nil {
		url := fmt.Sprintf("https://local-server/api/v1/request-schemas/%s/files/request-query.json", definitions.ID)
		querySchema, err := parse(c, url, definitions.RequestQuery)
		if err != nil {
			return nil, err
		}
		validator.requestQuery = querySchema
	}

	if definitions.RequestBody != nil {
		url := fmt.Sprintf("https://local-server/api/v1/request-schemas/%s/files/request-body.json", definitions.ID)
		bodySchema, err := parse(c, url, definitions.RequestBody)
		if err != nil {
			return nil, err
		}
		validator.requestBody = bodySchema
	}

	if definitions.ResponseHeaders != nil {
		url := fmt.Sprintf("https://local-server/api/v1/request-schemas/%s/files/response-headers.json", definitions.ID)
		headersSchema, err := parse(c, url, definitions.ResponseHeaders)
		if err != nil {
			return nil, err
		}
		validator.responseHeaders = headersSchema
	}

	if definitions.ResponseBody != nil {
		url := fmt.Sprintf("https://local-server/api/v1/request-schemas/%s/files/response-body.json", definitions.ID)
		bodySchema, err := parse(c, url, definitions.ResponseBody)
		if err != nil {
			return nil, err
		}
		validator.responseBody = bodySchema
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

func (v *Validator) ValidateHeaders(headers map[string][]string) error {
	if v.requestHeaders != nil {
		validationErr := v.requestHeaders.Validate(castToAny(headers))
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

func (v *Validator) ValidateQuery(query map[string][]string) error {
	if v.requestQuery != nil {
		validationErr := v.requestQuery.Validate(castToAny(query))
		if validationErr != nil {
			return validationErr
		}
	}
	return nil
}

func (v *Validator) ValidateBody(bodyReader io.Reader) error {
	if v.requestBody != nil {
		bodyMap, readErr := jsonschema.UnmarshalJSON(bodyReader)
		if readErr != nil {
			return readErr
		}

		validationErr := v.requestBody.Validate(bodyMap)
		if validationErr != nil {
			return validationErr
		}
	}
	return nil
}

func (v *Validator) ValidateResponseHeaders(headers map[string][]string) error {
	if v.responseHeaders != nil {
		validationErr := v.responseHeaders.Validate(castToAny(headers))
		if validationErr != nil {
			return validationErr
		}
	}
	return nil
}

func (v *Validator) ValidateResponseBody(bodyReader io.Reader) error {
	if v.responseBody != nil {
		bodyMap, readErr := jsonschema.UnmarshalJSON(bodyReader)
		if readErr != nil {
			return readErr
		}

		validationErr := v.responseBody.Validate(bodyMap)
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
