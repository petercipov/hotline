package servicelevels_test

import (
	"context"
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"hotline/concurrency"
	"hotline/integrations"
	"hotline/servicelevels"
	"strconv"
	"sync"
	"time"
)

var _ = Describe("SLO Pipeline", func() {
	sut := sloPipelineSUT{}
	It("should report no metrics if configuration not available", func() {
		sut.forPipeline()
		sut.NoConfigPresent()
		for i := 0; i < 1000; i++ {
			sut.IngestOKRequest(integrations.ID("unknown_integration_id-" + strconv.Itoa(i)))
		}
		reports := sut.Report()
		Expect(reports).To(HaveLen(sut.numberOfQueues))
		for _, report := range reports {
			Expect(report.Checks).To(BeEmpty())
		}
	})

	It("should report metrics if configuration available", func() {
		sut.forPipeline()
		for i := 0; i < 1000; i++ {
			sut.IngestOKRequest("known_integration_id")
		}
		reports := sut.Report()
		Expect(reports).To(HaveLen(sut.numberOfQueues))

		var nonEmptyReports []servicelevels.CheckReport
		for _, report := range reports {
			if len(report.Checks) > 0 {
				nonEmptyReports = append(nonEmptyReports, report)
			}
		}
		Expect(nonEmptyReports).To(HaveLen(1))
	})
})

type sloPipelineSUT struct {
	pipeline       *servicelevels.SLOPipeline
	sloRepository  *fakeSLORepository
	sloReporter    *fakeSLOReporter
	numberOfQueues int
}

func (s *sloPipelineSUT) forPipeline() {
	s.numberOfQueues = 8
	var queueIDs []string
	for i := 0; i < s.numberOfQueues; i++ {
		queueIDs = append(queueIDs, fmt.Sprintf("queue-%d", i))
	}
	scopes := concurrency.NewScopes(queueIDs, func(_ context.Context) *servicelevels.IntegrationsByQueue {
		return &servicelevels.IntegrationsByQueue{
			Integrations:     make(map[integrations.ID]*servicelevels.IntegrationSLO),
			LastObservedTime: time.Time{},
		}
	})

	s.sloRepository = &fakeSLORepository{}
	s.sloReporter = &fakeSLOReporter{}
	s.pipeline = servicelevels.NewSLOPipeline(
		s.sloRepository,
		s.sloReporter,
		scopes,
	)
}

func (s *sloPipelineSUT) NoConfigPresent() {
	s.sloRepository.NoConfig()
}

func (s *sloPipelineSUT) Report() []servicelevels.CheckReport {
	now := parseTime("2025-02-22T12:04:55Z")
	s.pipeline.Check(servicelevels.CheckMessage{
		Now: now,
	})

	for {
		reports := s.sloReporter.reports
		if len(reports) == s.numberOfQueues {
			return reports
		}

		time.Sleep(time.Millisecond * 1)
	}
}

func (s *sloPipelineSUT) IngestOKRequest(id integrations.ID) {
	now := parseTime("2025-02-22T12:04:05Z")
	s.pipeline.IngestHttpRequests(servicelevels.HttpReqsMessage{
		ID:  id,
		Now: now,
		Reqs: []*servicelevels.HttpRequest{
			{
				Latency: 1000,
				State:   "200",
				Method:  "GET",
				URL:     newUrl("https://test.com/api/"),
			},
		},
	})
}

type fakeSLOReporter struct {
	reports []servicelevels.CheckReport
	mux     sync.Mutex
}

func (f *fakeSLOReporter) ReportChecks(_ context.Context, report servicelevels.CheckReport) {
	f.mux.Lock()
	f.reports = append(f.reports, report)
	f.mux.Unlock()
}

type fakeSLORepository struct {
	noConfig bool
}

func (f *fakeSLORepository) GetIntegrationSLO(_ context.Context, integrationID integrations.ID) *servicelevels.IntegrationSLO {
	if f.noConfig {
		return nil
	}
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

func (f *fakeSLORepository) NoConfig() {
	f.noConfig = true
}
