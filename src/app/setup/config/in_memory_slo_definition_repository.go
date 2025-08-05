package config

import (
	"context"
	"hotline/integrations"
	"hotline/servicelevels"
	"sync"
)

type InMemorySLODefinitions struct {
	mutex  sync.Mutex
	config map[integrations.ID]servicelevels.HttpApiSLODefinition
}

func NewInMemorySLODefinitions() *InMemorySLODefinitions {
	return &InMemorySLODefinitions{
		config: map[integrations.ID]servicelevels.HttpApiSLODefinition{},
	}
}

func (f *InMemorySLODefinitions) GetConfig(_ context.Context, integrationID integrations.ID) *servicelevels.HttpApiSLODefinition {
	f.mutex.Lock()
	cfg, foundCfg := f.config[integrationID]
	f.mutex.Unlock()

	if !foundCfg {
		cfg = servicelevels.HttpApiSLODefinition{}
	}

	return &cfg
}

func (f *InMemorySLODefinitions) SetConfig(integrationID integrations.ID, cfg servicelevels.HttpApiSLODefinition) {
	f.mutex.Lock()
	f.config[integrationID] = cfg
	f.mutex.Unlock()
}
