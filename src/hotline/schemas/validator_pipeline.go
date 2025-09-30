package schemas

import (
	"context"
	"hotline/concurrency"
	hotlinehttp "hotline/http"
	"hotline/integrations"
	"io"
	"time"
)

type SchemaReader interface {
	GetSchemaContent(ctx context.Context, schema ID) io.ReadCloser
}

type ValidationReader interface {
	GetConfig(ctx context.Context, id integrations.ID) *ValidationDefinition
}

type ValidationReporter interface {
	Report(ctx context.Context, result ValidationResult)
}

type ValidatorScope struct {
	validators map[integrations.ID]*IntegrationValidation

	LastObservedTime time.Time
	schemaReader     SchemaReader
	validationReader ValidationReader

	validationReporter ValidationReporter
}

func NewEmptyValidatorScope(schemaRepo SchemaReader, validationRepo ValidationReader, reporter ValidationReporter) *ValidatorScope {
	return &ValidatorScope{
		validators: make(map[integrations.ID]*IntegrationValidation),

		LastObservedTime: time.Time{},
		schemaReader:     schemaRepo,
		validationReader: validationRepo,

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
		cfg := scope.validationReader.GetConfig(ctx, id)
		if cfg != nil {
			currentValidator = scope.buildRouteValidator(ctx, *cfg)
			scope.validators[id] = currentValidator
		}
	}
	return currentValidator
}

func (scope *ValidatorScope) buildRouteValidator(ctx context.Context, cfg ValidationDefinition) *IntegrationValidation {
	mux := &hotlinehttp.Mux[RouteValidator]{}
	for _, routeDef := range cfg.Routes {
		validator := scope.buildValidator(ctx, routeDef.SchemaDef)
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

func (scope *ValidatorScope) buildValidator(ctx context.Context, def RouteSchemaDefinition) *Validator {
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
		content := scope.schemaReader.GetSchemaContent(ctx, *id)
		if content != nil {
			defer func() {
				_ = content.Close()
			}()
			return &SchemaDefinition{
				ID:      *id,
				Content: content,
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
	fanOut *concurrency.FanOut[concurrency.ScopedAction[ValidatorScope], ValidatorScope]
}

func NewValidatorPipeline(scopes *concurrency.Scopes[ValidatorScope]) *ValidatorPipeline {
	return &ValidatorPipeline{
		fanOut: concurrency.NewActionFanOut(scopes),
	}
}

func (p *ValidatorPipeline) IngestHttpRequest(m *ValidateRequestMessage) {
	p.fanOut.Send(m.GetShardingKey(), m)
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

func (message *ValidateRequestMessage) GetShardingKey() []byte {
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

	scope.validationReporter.Report(ctx, result)
}
