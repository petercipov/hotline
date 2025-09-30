package schemas

import (
	"fmt"
	"hotline/http"
	"hotline/integrations"
	"hotline/uuid"
	"time"
)

type ID string

func (i *ID) String() string {
	return string(*i)
}

type IDGenerator func(now time.Time) (ID, error)

func NewIDGenerator(generator uuid.V7StringGenerator) IDGenerator {
	return func(now time.Time) (ID, error) {
		v7Str, err := generator(now)
		if err != nil {
			return "", err
		}
		return ID("SC" + v7Str), nil
	}
}

type ValidationDefinition struct {
	Routes []RouteValidationDefinition
}

type RouteValidationDefinition struct {
	Route     http.Route
	SchemaDef RouteSchemaDefinition
}

type RouteSchemaDefinition struct {
	Request *RequestSchemaDefinition
}

type RequestSchemaDefinition struct {
	HeaderSchemaID *ID
	QuerySchemaID  *ID
	BodySchemaID   *ID
}

type RequestPart string

const RequestHeaderPart RequestPart = "Request-Header"
const RequestQueryPart RequestPart = "Request-Query"
const RequestBodyPart RequestPart = "Request-Body"

type ValidationResult struct {
	RequestID     http.RequestID
	IntegrationID integrations.ID
	Errors        map[RequestPart]*ValidationError
	Success       map[RequestPart]ID
	Timestamp     time.Time
}

type ValidationError struct {
	SchemaID ID
	Err      error
}

func (v *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", v.SchemaID, v.Err.Error())
}
