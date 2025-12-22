package schemas

import (
	"context"
	"hotline/concurrency"
	hotlinehttp "hotline/http"
	"hotline/integrations"
	"io"
	"strings"
	"time"
)

type SchemaReader interface {
	GetSchema(ctx context.Context, id ID) (SchemaEntry, error)
}

type ValidationReader interface {
	GetValidations(ctx context.Context, id integrations.ID) ([]RouteValidationDefinition, error)
}

type ValidationReporter interface {
	HandleRequestValidated(ctx context.Context, result ValidationResult)
}

type ValidatorScope struct {
	validators map[integrations.ID]*IntegrationValidation

	LastObservedTime time.Time
	schemaReader     SchemaReader
	validationReader ValidationReader

	validationReporter ValidationReporter
}

func NewEmptyValidatorScope(schemaReader SchemaReader, validationReader ValidationReader, reporter ValidationReporter) *ValidatorScope {
	return &ValidatorScope{
		validators: make(map[integrations.ID]*IntegrationValidation),

		LastObservedTime: time.Time{},
		schemaReader:     schemaReader,
		validationReader: validationReader,

		validationReporter: reporter,
	}
}

func (scope *ValidatorScope) AdvanceTime(now time.Time) {
	if now.After(scope.LastObservedTime) {
		scope.LastObservedTime = now
	}
}

func (scope *ValidatorScope) ensureValidation(ctx context.Context, id integrations.ID) *IntegrationValidation {
	currentValidator, found := scope.validators[id]
	if !found {
		validations, getErr := scope.validationReader.GetValidations(ctx, id)
		if getErr == nil {
			currentValidator = scope.buildRouteValidator(ctx, validations)
			scope.validators[id] = currentValidator
		}
	}
	return currentValidator
}

func (scope *ValidatorScope) buildRouteValidator(ctx context.Context, routes []RouteValidationDefinition) *IntegrationValidation {
	mux := &hotlinehttp.Mux[RouteValidator]{}
	for _, routeDef := range routes {
		validator := scope.buildValidator(ctx, routeDef.Validators)
		if validator != nil {
			mux.Upsert(routeDef.Route, &RouteValidator{
				validator: validator,
			})
		}
	}
	return &IntegrationValidation{
		mux: mux,
	}
}

func (scope *ValidatorScope) buildValidator(ctx context.Context, def RouteValidators) *Validator {
	var requestSchema RequestSchema
	if def.Request != nil {
		requestSchema.RequestHeaders = scope.getSchemaDefinition(ctx, def.Request.HeaderSchemaID)
		requestSchema.RequestQuery = scope.getSchemaDefinition(ctx, def.Request.QuerySchemaID)
		requestSchema.RequestBody = scope.getSchemaDefinition(ctx, def.Request.BodySchemaID)
	}

	validator, _ := NewRequestValidator(requestSchema)
	return validator
}

func (scope *ValidatorScope) getSchemaDefinition(ctx context.Context, id *ID) *SchemaDefinition {
	if id != nil {
		entry, getErr := scope.schemaReader.GetSchema(ctx, *id)
		if getErr == nil {
			return &SchemaDefinition{
				ID:      *id,
				Content: strings.NewReader(entry.Content),
			}
		}
	}
	return nil
}

type IntegrationValidation struct {
	mux *hotlinehttp.Mux[RouteValidator]
}

func (v IntegrationValidation) ValidateRequest(request RequestContent) (map[RequestPart]ID, map[RequestPart]*ValidationError) {
	handler := v.mux.LocaleHandler(request.Locator)
	if handler == nil {
		return nil, nil
	}
	errors := make(map[RequestPart]*ValidationError, 3)
	success := make(map[RequestPart]ID, 3)
	schemaID, headerErr := handler.validator.ValidateHeaders(request.Headers)
	if headerErr != nil {
		errors[RequestHeaderPart] = headerErr
	} else if schemaID != nil {
		success[RequestHeaderPart] = *schemaID
	}
	schemaID, queryErr := handler.validator.ValidateQuery(request.Query)
	if queryErr != nil {
		errors[RequestQueryPart] = queryErr
	} else if schemaID != nil {
		success[RequestQueryPart] = *schemaID
	}
	schemaID, bodyErr := handler.validator.ValidateBody(request.Body)
	if bodyErr != nil {
		errors[RequestBodyPart] = bodyErr
	} else if schemaID != nil {
		success[RequestBodyPart] = *schemaID
	}
	return success, errors
}

type RouteValidator struct {
	validator *Validator
}

type ValidatorPipeline struct {
	publisher concurrency.PartitionPublisher
}

func NewValidatorPipeline(publisher concurrency.PartitionPublisher) *ValidatorPipeline {
	return &ValidatorPipeline{
		publisher: publisher,
	}
}

func (p *ValidatorPipeline) IngestHttpRequest(ctx context.Context, m *ValidateRequestMessage) {
	p.publisher.PublishToPartition(ctx, m)
}

type RequestContent struct {
	Locator hotlinehttp.RequestLocator
	Headers map[string][]string
	Query   map[string][]string
	Body    io.Reader
}

type ValidateRequestMessage struct {
	ID            hotlinehttp.RequestID
	IntegrationID integrations.ID
	Now           time.Time

	Request RequestContent
}

func (message *ValidateRequestMessage) GetShardingKey() concurrency.ShardingKey {
	return []byte(message.IntegrationID)
}

func (message *ValidateRequestMessage) Execute(ctx context.Context, _ string, scope *ValidatorScope) {
	scope.AdvanceTime(message.Now)
	result := ValidationResult{
		IntegrationID: message.IntegrationID,
		RequestID:     message.ID,
		Timestamp:     message.Now,
	}
	validation := scope.ensureValidation(ctx, message.IntegrationID)
	if validation != nil {
		success, errors := validation.ValidateRequest(message.Request)
		result.Errors = errors
		result.Success = success
	}

	scope.validationReporter.HandleRequestValidated(ctx, result)
}
