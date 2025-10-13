package servicelevels

import (
	"context"
	"hotline/integrations"
	"sync"
)

type InMemoryRepository struct {
	configs map[integrations.ID]*ApiServiceLevels
	mux     sync.Mutex
}

func (i *InMemoryRepository) GetServiceLevels(_ context.Context, id integrations.ID) (ApiServiceLevels, error) {
	i.mux.Lock()
	defer i.mux.Unlock()
	sloConf, found := i.configs[id]
	if !found {
		return ApiServiceLevels{}, ErrServiceLevelsNotFound
	}

	return *sloConf, nil
}

func (i *InMemoryRepository) Modify(_ context.Context, id integrations.ID, slo ApiServiceLevels) error {
	i.mux.Lock()
	defer i.mux.Unlock()
	if i.configs == nil {
		i.configs = make(map[integrations.ID]*ApiServiceLevels)
	}

	i.configs[id] = &slo

	return nil
}

func (i *InMemoryRepository) Drop(_ context.Context, id integrations.ID) error {
	i.mux.Lock()
	defer i.mux.Unlock()
	delete(i.configs, id)
	return nil
}

type InMemorySLOReporter struct {
	reports []CheckReport
	mux     sync.Mutex
}

func (f *InMemorySLOReporter) ReportChecks(_ context.Context, report CheckReport) {
	f.mux.Lock()
	defer f.mux.Unlock()
	f.reports = append(f.reports, report)
}

func (f *InMemorySLOReporter) GetReports() ReportArr {
	f.mux.Lock()
	defer f.mux.Unlock()
	return f.reports
}

type InMemoryEventPublisher struct {
	arr []ModifyForRouteMessage
	mux sync.Mutex
}

func (p *InMemoryEventPublisher) HandleRouteModified(event []ModifyForRouteMessage) error {
	p.mux.Lock()
	defer p.mux.Unlock()
	p.arr = append(p.arr, event...)
	return nil
}

type ReportArr []CheckReport

func (r ReportArr) GroupByIntegrationID() map[integrations.ID][]LevelsCheck {
	byIntegrationID := make(map[integrations.ID][]LevelsCheck)
	for _, checks := range r {
		for _, check := range checks {
			byIntegrationID[check.IntegrationID] = append(byIntegrationID[check.IntegrationID], check.Levels...)
		}
	}
	return byIntegrationID
}
