package schemas

import (
	"context"
	"hotline/http"
	"hotline/integrations"
	"hotline/uuid"
	"io"
	"slices"
	"strings"
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
	mutext      sync.Mutex
	schemas     map[ID]inMemorySchemaEntry
	idGenerator IDGenerator
}

func NewInMemorySchemaRepository(generator uuid.V7StringGenerator) *InMemorySchemaRepository {
	return &InMemorySchemaRepository{
		schemas:     make(map[ID]inMemorySchemaEntry),
		idGenerator: NewIDGenerator(generator),
	}
}

func (r *InMemorySchemaRepository) GenerateID(now time.Time) (ID, error) {
	return r.idGenerator(now)
}

func (r *InMemorySchemaRepository) GetSchemaByID(_ context.Context, schemaID ID) (SchemaEntry, error) {
	r.mutext.Lock()
	defer r.mutext.Unlock()

	var result SchemaEntry
	var resErr = io.EOF
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

func (r *InMemorySchemaRepository) GetSchemaContent(_ context.Context, schemaID ID) io.ReadCloser {
	r.mutext.Lock()
	defer r.mutext.Unlock()

	var result io.ReadCloser
	entry, found := r.schemas[schemaID]
	if found {
		result = io.NopCloser(strings.NewReader(entry.content))
	}
	return result
}

func (r *InMemorySchemaRepository) SetSchema(_ context.Context, id ID, content string, updatedAt time.Time, title string) error {
	r.mutext.Lock()
	defer r.mutext.Unlock()

	compiler := createCompiler()
	_, validatorErr := newJsonSchemaValidator(
		SchemaDefinition{
			ID:      id,
			Content: strings.NewReader(content),
		},
		"test",
		compiler,
	)

	if validatorErr != nil {
		return &ValidationError{
			SchemaID: id,
			Err:      validatorErr,
		}
	}

	r.schemas[id] = inMemorySchemaEntry{
		id:        id,
		content:   content,
		title:     title,
		updatedAt: updatedAt,
	}
	return nil
}

func (r *InMemorySchemaRepository) ListSchemas(_ context.Context) []SchemaListEntry {
	r.mutext.Lock()
	defer r.mutext.Unlock()

	var entries []SchemaListEntry
	for id, entry := range r.schemas {
		entries = append(entries, SchemaListEntry{
			ID:        id,
			Title:     entry.title,
			UpdatedAt: entry.updatedAt,
		})
	}
	return entries
}

func (r *InMemorySchemaRepository) DeleteSchema(_ context.Context, schemaID ID) error {
	r.mutext.Lock()
	defer r.mutext.Unlock()

	var resErr = io.EOF
	_, found := r.schemas[schemaID]
	if found {
		resErr = nil
		delete(r.schemas, schemaID)
	}
	return resErr
}

type InMemoryValidationRepository struct {
	mutex   sync.Mutex
	mapping map[integrations.ID]*ValidationDefinition
}

func NewInMemoryValidationRepository() *InMemoryValidationRepository {
	return &InMemoryValidationRepository{
		mapping: make(map[integrations.ID]*ValidationDefinition),
	}
}

func (r *InMemoryValidationRepository) GetConfig(_ context.Context, id integrations.ID) *ValidationDefinition {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	var result *ValidationDefinition
	definitions, found := r.mapping[id]
	if found {
		result = definitions
	}
	return result
}

func (r *InMemoryValidationRepository) SetConfig(_ context.Context, id integrations.ID, route http.Route, schemaDef RouteSchemaDefinition) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	byIntegration, found := r.mapping[id]
	if !found {
		byIntegration = &ValidationDefinition{}
		r.mapping[id] = byIntegration
	}

	byIntegration.Routes = slices.DeleteFunc(byIntegration.Routes, func(def RouteValidationDefinition) bool {
		return def.Route == route
	})

	byIntegration.Routes = append(byIntegration.Routes, RouteValidationDefinition{
		Route:     route,
		SchemaDef: schemaDef,
	})
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
