package schemas

import (
	"context"
	"hotline/integrations"
	"io"
	"strings"
	"sync"
)

type InMemorySchemaRepository struct {
	mutext  sync.Mutex
	schemas map[ID]string
}

func (r *InMemorySchemaRepository) GetSchema(_ context.Context, schemaID ID) io.ReadCloser {
	r.mutext.Lock()
	defer r.mutext.Unlock()

	var result io.ReadCloser
	schemas, found := r.schemas[schemaID]
	if found {
		result = io.NopCloser(strings.NewReader(schemas))
	}
	return result
}

func (r *InMemorySchemaRepository) SetSchema(id ID, content string) {
	r.mutext.Lock()
	defer r.mutext.Unlock()

	if r.schemas == nil {
		r.schemas = make(map[ID]string)
	}
	r.schemas[id] = content
}

type InMemoryValidationRepository struct {
	mutext  sync.Mutex
	mapping map[integrations.ID]*ValidationDefinition
}

func (r *InMemoryValidationRepository) GetConfig(_ context.Context, id integrations.ID) *ValidationDefinition {
	r.mutext.Lock()
	defer r.mutext.Unlock()

	var result *ValidationDefinition
	definitions, found := r.mapping[id]
	if found {
		result = definitions
	}
	return result
}

func (r *InMemoryValidationRepository) SetConfig(id integrations.ID, definition *ValidationDefinition) {
	r.mutext.Lock()
	defer r.mutext.Unlock()

	if r.mapping == nil {
		r.mapping = make(map[integrations.ID]*ValidationDefinition)
	}

	r.mapping[id] = definition
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
