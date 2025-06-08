package config

import (
	"context"
	"hotline/integrations"
	"hotline/servicelevels"
	"sync"
	"time"
)

type FakeSLOConfigRepository struct {
	mutex  sync.Mutex
	config map[integrations.ID]servicelevels.HttpApiSLODefinition
}

func NewFakeSLOConfigRepository() *FakeSLOConfigRepository {
	return &FakeSLOConfigRepository{
		config: map[integrations.ID]servicelevels.HttpApiSLODefinition{},
	}
}

func (f *FakeSLOConfigRepository) GetIntegrationSLO(_ context.Context, integrationID integrations.ID) *servicelevels.IntegrationSLO {
	f.mutex.Lock()
	cfg, foundCfg := f.config[integrationID]
	f.mutex.Unlock()

	if !foundCfg {
		cfg = servicelevels.HttpApiSLODefinition{
			RouteSLOs: []servicelevels.HttpRouteSLODefinition{defaultRouteDefinition("", "", "/")},
		}
	}

	apiSLO, createErr := servicelevels.NewHttpApiSLO(cfg)
	if createErr != nil {
		return nil
	}

	return &servicelevels.IntegrationSLO{
		ID:         integrationID,
		HttpApiSLO: apiSLO,
	}
}

func (f *FakeSLOConfigRepository) SetConfig(integrationID integrations.ID, cfg servicelevels.HttpApiSLODefinition) {
	f.mutex.Lock()
	f.config[integrationID] = cfg
	f.mutex.Unlock()
}

func defaultRouteDefinition(method string, host string, path string) servicelevels.HttpRouteSLODefinition {
	return servicelevels.HttpRouteSLODefinition{
		Method: method,
		Path:   path,
		Host:   host,
		Latency: servicelevels.HttpLatencySLODefinition{
			Percentiles: []servicelevels.PercentileDefinition{
				{
					Percentile: 99.9,
					Threshold:  2000,
					Name:       "p99",
				},
			},
			WindowDuration: 1 * time.Minute,
		},
		Status: servicelevels.HttpStatusSLODefinition{
			Expected:        []string{"200", "201"},
			BreachThreshold: 99.9,
			WindowDuration:  1 * time.Hour,
		},
	}
}
