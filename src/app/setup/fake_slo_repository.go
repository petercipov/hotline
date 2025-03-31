package setup

import (
	"context"
	"hotline/integrations"
	"hotline/servicelevels"
	"time"
)

type FakeSLOConfigRepository struct {
}

func (f *FakeSLOConfigRepository) GetIntegrationSLO(_ context.Context, integrationID integrations.ID) *servicelevels.IntegrationSLO {
	apiSLO, createErr := servicelevels.NewHttpApiSLO(servicelevels.HttpApiSLODefinition{
		RouteSLOs: []servicelevels.HttpRouteSLODefinition{defaultRouteDefinition("", "/")},
	})
	if createErr != nil {
		return nil
	}

	return &servicelevels.IntegrationSLO{
		ID:         integrationID,
		HttpApiSLO: apiSLO,
	}
}

func defaultRouteDefinition(host string, path string) servicelevels.HttpRouteSLODefinition {
	return servicelevels.HttpRouteSLODefinition{
		Path: path,
		Host: host,
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
