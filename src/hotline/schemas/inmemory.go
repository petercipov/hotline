package schemas

import (
	"context"
	"hotline/http"
	"hotline/integrations"
	"slices"
	"sync"
	"time"
)

type SchemaEntry struct {
	ID        ID
	Content   string
	Title     string
	UpdatedAt time.Time
}

type SchemaListEntry struct {
	ID        ID
	Title     string
	UpdatedAt time.Time
}

type inMemorySchemaEntry struct {
	id        ID
	content   string
	title     string
	updatedAt time.Time
}
type InMemorySchemaRepository struct {
	mutex   sync.Mutex
	schemas map[ID]inMemorySchemaEntry
}

func NewInMemorySchemaRepository() *InMemorySchemaRepository {
	return &InMemorySchemaRepository{
		schemas: make(map[ID]inMemorySchemaEntry),
	}
}

func (r *InMemorySchemaRepository) GetSchema(_ context.Context, schemaID ID) (SchemaEntry, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	var result SchemaEntry
	var resErr = ErrSchemaNotFound
	entry, found := r.schemas[schemaID]
	if found {
		result = SchemaEntry{
			ID:        entry.id,
			Content:   entry.content,
			Title:     entry.title,
			UpdatedAt: entry.updatedAt,
		}
		resErr = nil
	}
	return result, resErr
}

func (r *InMemorySchemaRepository) GetSchemaEntry(ctx context.Context, id ID) (SchemaListEntry, error) {
	schema, getErr := r.GetSchema(ctx, id)
	var entry SchemaListEntry
	if getErr == nil {
		entry = SchemaListEntry{
			ID:        schema.ID,
			Title:     schema.Title,
			UpdatedAt: schema.UpdatedAt,
		}
	}
	return entry, getErr
}

func (r *InMemorySchemaRepository) SetSchema(_ context.Context, id ID, content string, updatedAt time.Time, title string) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	r.schemas[id] = inMemorySchemaEntry{
		id:        id,
		content:   content,
		title:     title,
		updatedAt: updatedAt,
	}
	return nil
}

func (r *InMemorySchemaRepository) ListSchemas(_ context.Context) ([]SchemaListEntry, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	var entries []SchemaListEntry
	for id, entry := range r.schemas {
		entries = append(entries, SchemaListEntry{
			ID:        id,
			Title:     entry.title,
			UpdatedAt: entry.updatedAt,
		})
	}
	return entries, nil
}

func (r *InMemorySchemaRepository) DeleteSchema(_ context.Context, schemaID ID) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	var resErr = ErrSchemaNotFound
	_, found := r.schemas[schemaID]
	if found {
		resErr = nil
		delete(r.schemas, schemaID)
	}
	return resErr
}

type inMemValidations struct {
	routes []RouteValidationDefinition
}

type InMemoryValidationRepository struct {
	mutex   sync.Mutex
	mapping map[integrations.ID]*inMemValidations
}

func NewInMemoryValidationRepository() *InMemoryValidationRepository {
	return &InMemoryValidationRepository{
		mapping: make(map[integrations.ID]*inMemValidations),
	}
}

func (r *InMemoryValidationRepository) GetValidations(_ context.Context, id integrations.ID) ([]RouteValidationDefinition, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	var result []RouteValidationDefinition
	definitions, found := r.mapping[id]
	if found {
		result = definitions.routes
	}
	return result, nil
}

func (r *InMemoryValidationRepository) SetForRoute(_ context.Context, id integrations.ID, routeKey http.RouteKey, route http.Route, schemaDef RouteValidators) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	byIntegration, found := r.mapping[id]
	if !found {
		byIntegration = &inMemValidations{}
		r.mapping[id] = byIntegration
	}

	byIntegration.routes = slices.DeleteFunc(byIntegration.routes, func(def RouteValidationDefinition) bool {
		return def.Route == route
	})

	byIntegration.routes = append(byIntegration.routes, RouteValidationDefinition{
		Route:      route,
		RouteKey:   routeKey,
		Validators: schemaDef,
	})
	return nil
}

func (r *InMemoryValidationRepository) DeleteRouteByKey(_ context.Context, id integrations.ID, key http.RouteKey) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	byIntegration, found := r.mapping[id]
	if found {
		byIntegration.routes = slices.DeleteFunc(byIntegration.routes, func(def RouteValidationDefinition) bool {
			return def.Route.GenerateKey(id.String()) == key
		})
	}
	return nil
}

type InMemoryValidationReporter struct {
	results []ValidationResult
	mutext  sync.Mutex
}

func (r *InMemoryValidationReporter) Report(_ context.Context, res ValidationResult) {
	r.mutext.Lock()
	defer r.mutext.Unlock()
	r.results = append(r.results, res)
}

func (r *InMemoryValidationReporter) GetResults() []ValidationResult {
	r.mutext.Lock()
	defer r.mutext.Unlock()
	return r.results
}
