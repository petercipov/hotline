package config

import (
	"context"
	"hotline/http"
	"hotline/integrations"
	"hotline/servicelevels"
	"sync"
	"time"
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
		cfg = servicelevels.HttpApiSLODefinition{
			Routes: []servicelevels.HttpRouteSLODefinition{defaultRouteDefinition("", "", "/")},
		}
	}

	return &cfg
}

func (f *InMemorySLODefinitions) SetConfig(integrationID integrations.ID, cfg servicelevels.HttpApiSLODefinition) {
	f.mutex.Lock()
	f.config[integrationID] = cfg
	f.mutex.Unlock()
}

func defaultRouteDefinition(method string, host string, pathPattern string) servicelevels.HttpRouteSLODefinition {
	return servicelevels.HttpRouteSLODefinition{
		Route: http.Route{
			Method:      method,
			PathPattern: pathPattern,
			Host:        host,
			Port:        0,
		},
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
