package servicelevels_test

import (
	"context"
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"hotline/clock"
	"hotline/concurrency"
	"hotline/http"
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

		for i := 0; i < 1000; i++ {
			id := integrations.ID("unknown_integration_id-" + strconv.Itoa(i))
			sut.NoConfigPresent(id, "2025-02-22T12:04:00Z")
			sut.IngestOKRequest(id, "2025-02-22T12:04:05Z")
		}
		reports := sut.Report("2025-02-22T12:04:55Z")
		Expect(reports).To(HaveLen(sut.numberOfQueues))
		for _, report := range reports {
			Expect(report.Checks).To(BeEmpty())
		}
	})

	It("should report metrics if configuration available", func() {
		sut.forPipeline()
		sut.ForDefaultConfig("known_integration_id", "2025-02-22T12:04:00Z")
		for i := 0; i < 1000; i++ {
			sut.IngestOKRequest("known_integration_id", "2025-02-22T12:04:05Z")
		}
		reports := sut.Report("2025-02-22T12:04:55Z")
		Expect(reports).To(HaveLen(sut.numberOfQueues))

		var nonEmptyReports []*servicelevels.CheckReport
		for _, report := range reports {
			if len(report.Checks) > 0 {
				nonEmptyReports = append(nonEmptyReports, report)
			}
		}
		Expect(nonEmptyReports).To(HaveLen(1))
	})

	Context("updating slo config", func() {
		It("should report different config if route was changed", func() {
			sut.forPipeline()
			sut.ForDefaultConfig("known_integration_id", "2025-02-22T12:04:00Z")
			for i := 0; i < 1000; i++ {
				sut.IngestOKRequest("known_integration_id", "2025-02-22T12:04:05Z")
			}

			sut.ChangeConfig("known_integration_id", "2025-02-22T12:05:05Z")

			reports := sut.Report("2025-02-22T12:05:05Z")
			Expect(reports).To(HaveLen(sut.numberOfQueues))

			var nonEmptyReports []*servicelevels.CheckReport
			for _, report := range reports {
				if len(report.Checks) > 0 {
					nonEmptyReports = append(nonEmptyReports, report)
				}
			}
			Expect(nonEmptyReports).To(HaveLen(1))
		})

		It("should report nothing config was removed", func() {
			sut.forPipeline()
			sut.ForDefaultConfig("known_integration_id", "2025-02-22T12:04:00Z")
			for i := 0; i < 10; i++ {
				sut.IngestOKRequest("known_integration_id", "2025-02-22T12:04:05Z")
			}
			sut.NoConfigPresent("known_integration_id", "2025-02-22T12:04:05Z")

			reports := sut.Report("2025-02-22T12:05:05Z")
			Expect(reports).To(HaveLen(sut.numberOfQueues))

			for _, report := range reports {
				Expect(report.Checks).To(BeEmpty())
			}
		})
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

	s.sloRepository = &fakeSLORepository{
		configs: make(map[integrations.ID]*servicelevels.HttpApiSLODefinition),
	}
	s.sloReporter = &fakeSLOReporter{}

	scopes := concurrency.NewScopes(queueIDs, func(_ context.Context) *servicelevels.IntegrationsScope {
		return servicelevels.NewEmptyIntegrationsScope(s.sloRepository, s.sloReporter)
	})
	s.pipeline = servicelevels.NewSLOPipeline(
		scopes,
	)
}

func (s *sloPipelineSUT) NoConfigPresent(id integrations.ID, timeStr string) {
	s.sloRepository.NoConfig(id)

	now := clock.ParseTime(timeStr)
	s.pipeline.ModifyRoute(&servicelevels.ModifyRouteMessage{
		ID:  id,
		Now: now,

		Route: http.Route{
			PathPattern: "/",
		},
	})
}

func (s *sloPipelineSUT) Report(timeStr string) []*servicelevels.CheckReport {
	now := clock.ParseTime(timeStr)
	s.pipeline.Check(&servicelevels.CheckMessage{
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

func (s *sloPipelineSUT) IngestOKRequest(id integrations.ID, timeStr string) {
	now := clock.ParseTime(timeStr)
	s.pipeline.IngestHttpRequest(&servicelevels.IngestRequestsMessage{
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

func (s *sloPipelineSUT) ChangeConfig(integrationID integrations.ID, timeStr string) {
	now := clock.ParseTime(timeStr)

	s.sloRepository.SetConfig(integrationID, &servicelevels.HttpApiSLODefinition{
		RouteSLOs: []servicelevels.HttpRouteSLODefinition{defaultRouteDefinition("", "/")},
	})

	s.pipeline.ModifyRoute(&servicelevels.ModifyRouteMessage{
		ID:  integrationID,
		Now: now,

		Route: http.Route{
			PathPattern: "/",
		},
	})
}

func (s *sloPipelineSUT) ForDefaultConfig(integrationID integrations.ID, timeStr string) {
	now := clock.ParseTime(timeStr)

	s.sloRepository.SetConfig(integrationID, &servicelevels.HttpApiSLODefinition{
		RouteSLOs: []servicelevels.HttpRouteSLODefinition{defaultRouteDefinition("", "/")},
	})

	s.pipeline.ModifyRoute(&servicelevels.ModifyRouteMessage{
		ID:  integrationID,
		Now: now,

		Route: http.Route{
			PathPattern: "/",
		},
	})
}

type fakeSLOReporter struct {
	reports []*servicelevels.CheckReport
	mux     sync.Mutex
}

func (f *fakeSLOReporter) ReportChecks(_ context.Context, report *servicelevels.CheckReport) {
	f.mux.Lock()
	f.reports = append(f.reports, report)
	f.mux.Unlock()
}

type fakeSLORepository struct {
	configs map[integrations.ID]*servicelevels.HttpApiSLODefinition
}

func (f *fakeSLORepository) GetConfig(_ context.Context, id integrations.ID) *servicelevels.HttpApiSLODefinition {

	sloConf, found := f.configs[id]
	if !found {
		return nil
	}

	return sloConf
}

func (f *fakeSLORepository) SetConfig(id integrations.ID, slo *servicelevels.HttpApiSLODefinition) {
	f.configs[id] = slo
}

func (f *fakeSLORepository) NoConfig(id integrations.ID) {
	delete(f.configs, id)
}
