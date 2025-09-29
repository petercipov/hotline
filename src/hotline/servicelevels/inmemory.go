package servicelevels

import (
	"context"
	"hotline/integrations"
	"sync"
)

type InMemorySLORepository struct {
	configs map[integrations.ID]*HttpApiServiceLevels
	mux     sync.Mutex
}

func (i *InMemorySLORepository) GetConfig(_ context.Context, id integrations.ID) *HttpApiServiceLevels {
	i.mux.Lock()
	defer i.mux.Unlock()
	sloConf, found := i.configs[id]
	if !found {
		return nil
	}

	return sloConf
}

func (i *InMemorySLORepository) SetConfig(_ context.Context, id integrations.ID, slo *HttpApiServiceLevels) {
	i.mux.Lock()
	defer i.mux.Unlock()
	if i.configs == nil {
		i.configs = make(map[integrations.ID]*HttpApiServiceLevels)
	}

	if slo == nil {
		delete(i.configs, id)
	} else {
		i.configs[id] = slo
	}
}

type InMemorySLOReporter struct {
	reports []*CheckReport
	mux     sync.Mutex
}

func (f *InMemorySLOReporter) ReportChecks(_ context.Context, report *CheckReport) {
	f.mux.Lock()
	defer f.mux.Unlock()
	f.reports = append(f.reports, report)
}

func (f *InMemorySLOReporter) GetReports() []*CheckReport {
	f.mux.Lock()
	defer f.mux.Unlock()

	return f.reports
}
