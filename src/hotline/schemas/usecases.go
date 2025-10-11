package schemas

import (
	"context"
	"errors"
	"hotline/clock"
	"hotline/http"
	"hotline/integrations"
	"hotline/uuid"
	"sort"
	"strings"
	"time"
)

var ErrSchemaNotFound = errors.New("schema not found")

type SchemaRepository interface {
	SchemaReader
	GetSchemaEntry(ctx context.Context, id ID) (SchemaListEntry, error)
	SetSchema(ctx context.Context, id ID, content string, updateAt time.Time, title string) error
	ListSchemas(ctx context.Context) ([]SchemaListEntry, error)
	DeleteSchema(ctx context.Context, id ID) error
}

type ValidationRepository interface {
	ValidationReader
	SetForRoute(ctx context.Context, id integrations.ID, routeKey http.RouteKey, route http.Route, schemaDef RouteValidators) error
	DeleteRouteByKey(ctx context.Context, id integrations.ID, key http.RouteKey) error
}

type SchemaUseCase struct {
	repository  SchemaRepository
	nowFunc     clock.NowFunc
	idGenerator IDGenerator
}

func NewSchemaUseCase(repository SchemaRepository, nowFunc clock.NowFunc, generator uuid.V7StringGenerator) *SchemaUseCase {
	return &SchemaUseCase{
		repository:  repository,
		nowFunc:     nowFunc,
		idGenerator: NewIDGenerator(generator),
	}
}

func (u *SchemaUseCase) GetSchema(ctx context.Context, schemaID ID) (SchemaEntry, error) {
	return u.repository.GetSchema(ctx, schemaID)
}

func (u *SchemaUseCase) ListSchemas(ctx context.Context) ([]SchemaListEntry, error) {
	entries, listErr := u.repository.ListSchemas(ctx)
	if listErr == nil {
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].UpdatedAt.Before(entries[j].UpdatedAt)
		})
	}
	return entries, listErr
}

func (u *SchemaUseCase) DeleteSchema(ctx context.Context, schemaID ID) error {
	entry, getErr := u.repository.GetSchemaEntry(ctx, schemaID)
	if getErr != nil {
		return getErr
	}
	return u.repository.DeleteSchema(ctx, entry.ID)
}

func (u *SchemaUseCase) CreateSchema(ctx context.Context, content string, title string) (SchemaListEntry, error) {
	now := u.nowFunc()
	schemaID, generateErr := u.idGenerator(now)
	if generateErr != nil {
		return SchemaListEntry{}, generateErr
	}

	validationErr := u.validateSchemaContent(schemaID, content)
	if validationErr != nil {
		return SchemaListEntry{}, validationErr
	}

	var entry SchemaListEntry
	var opErr error
	opErr = u.repository.SetSchema(ctx, schemaID, content, now, title)
	if opErr == nil {
		entry, opErr = u.repository.GetSchemaEntry(ctx, schemaID)
	}
	return entry, opErr
}

func (u *SchemaUseCase) ModifySchema(ctx context.Context, schemaID ID, content string, title string) error {
	now := u.nowFunc()

	validationErr := u.validateSchemaContent(schemaID, content)
	if validationErr != nil {
		return validationErr
	}

	entry, getErr := u.repository.GetSchemaEntry(ctx, schemaID)
	if getErr != nil {
		return getErr
	}

	return u.repository.SetSchema(ctx, entry.ID, content, now, title)
}

func (u *SchemaUseCase) validateSchemaContent(id ID, content string) error {
	compiler := createCompiler()
	_, validatorErr := newJsonSchemaValidator(
		SchemaDefinition{
			ID:      id,
			Content: strings.NewReader(content),
		},
		"validation-test",
		compiler,
	)

	if validatorErr != nil {
		return &ValidationError{
			SchemaID: id,
			Err:      validatorErr,
		}
	}
	return nil
}

type ValidationUseCase struct {
	repository ValidationRepository
	schemaRepo SchemaRepository
}

func NewValidationUseCase(repository ValidationRepository, schemaRepo SchemaRepository) *ValidationUseCase {
	return &ValidationUseCase{
		repository: repository,
		schemaRepo: schemaRepo,
	}
}

func (u *ValidationUseCase) UpsertValidation(ctx context.Context, id integrations.ID, route http.Route, validators RouteValidators) (http.RouteKey, error) {
	route = route.Normalize()
	routeKey := route.GenerateKey(id.String())

	schemaIds := []*ID{
		validators.Request.BodySchemaID,
		validators.Request.QuerySchemaID,
		validators.Request.HeaderSchemaID,
	}
	for _, schemaId := range schemaIds {
		if schemaId == nil {
			continue
		}
		_, getErr := u.schemaRepo.GetSchema(ctx, *schemaId)
		if getErr != nil {
			return routeKey, getErr
		}
	}

	return routeKey, u.repository.SetForRoute(ctx, id, routeKey, route, validators)
}

func (u *ValidationUseCase) DeleteValidation(ctx context.Context, id integrations.ID, key http.RouteKey) error {
	return u.repository.DeleteRouteByKey(ctx, id, key)
}

func (u *ValidationUseCase) GetValidations(ctx context.Context, id integrations.ID) ([]RouteValidationDefinition, error) {
	return u.repository.GetValidations(ctx, id)
}
