package servicelevels_test

import (
	"context"
	"hotline/clock"
	"hotline/concurrency"
	"hotline/http"
	"hotline/integrations"
	"hotline/servicelevels"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Service Levels Pipeline", func() {
	sut := sloPipelineSUT{}

	BeforeEach(func() {
		sut.Close()
	})

	It("should report no metrics if configuration not available", func() {
		sut.forPipeline()

		for i := range 1000 {
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
		for range 1000 {
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
			for range 1000 {
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

		It("should report nothing when config was removed", func() {
			sut.forPipeline()
			sut.ForDefaultConfig("known_integration_id", "2025-02-22T12:04:00Z")
			for range 10 {
				sut.IngestOKRequest("known_integration_id", "2025-02-22T12:04:05Z")
			}
			sut.NoConfigPresent("known_integration_id", "2025-02-22T12:04:05Z")

			reports := sut.Report("2025-02-22T12:05:05Z")
			Expect(reports).To(HaveLen(sut.numberOfQueues))

			for _, report := range reports {
				Expect(report.Checks).To(BeEmpty())
			}
		})

		It("should report nothing when config was emptied", func() {
			sut.forPipeline()
			sut.ForDefaultConfig("known_integration_id", "2025-02-22T12:04:00Z")
			for range 10 {
				sut.IngestOKRequest("known_integration_id", "2025-02-22T12:04:05Z")
			}
			sut.EmptyConfigPresent("known_integration_id", "2025-02-22T12:04:05Z")

			reports := sut.Report("2025-02-22T12:05:05Z")
			Expect(reports).To(HaveLen(sut.numberOfQueues))

			for _, report := range reports {
				if len(report.Checks) == 1 {
					Expect(report.Checks[0].SLO).To(BeEmpty())
					Expect(string(report.Checks[0].IntegrationID)).To(Equal("known_integration_id"))
				} else {
					Expect(report.Checks).To(BeEmpty())
				}
			}
		})
	})

	It("should report less when config is reduced from multiple to single slo", func() {
		sut.forPipeline()
		sut.ForMultipleConfig("known_integration_id", "2025-02-22T12:04:00Z")

		sut.IngestOKRequestToUrl("known_integration_id", "2025-02-22T12:04:05Z", "/api")
		sut.IngestOKRequestToUrl("known_integration_id", "2025-02-22T12:04:05Z", "/products")
		sut.IngestOKRequestToUrl("known_integration_id", "2025-02-22T12:04:05Z", "/orders")

		sut.DropNonDefaultRoutes("known_integration_id")

		reports := sut.Report("2025-02-22T12:05:05Z")
		Expect(reports).To(HaveLen(sut.numberOfQueues))

		checks := 0
		for _, report := range reports {
			checks += len(report.Checks)
		}
		Expect(checks).To(Equal(1))
	})
})

type sloPipelineSUT struct {
	pipeline      *servicelevels.Pipeline
	sloRepository *servicelevels.InMemoryRepository
	sloReporter   *servicelevels.InMemorySLOReporter
	eventsHandler *servicelevels.EventsHandler

	useCase        *servicelevels.UseCase
	manualClock    *clock.ManualClock
	numberOfQueues int
}

func (s *sloPipelineSUT) forPipeline() {
	s.manualClock = clock.NewManualClock(
		clock.ParseTime("2025-02-22T12:02:10Z"),
		500*time.Microsecond,
	)

	s.numberOfQueues = 8
	queueIDs := concurrency.GenerateScopeIds("queue", s.numberOfQueues)
	s.sloRepository = &servicelevels.InMemoryRepository{}
	s.sloReporter = &servicelevels.InMemorySLOReporter{}
	s.eventsHandler = &servicelevels.EventsHandler{}

	s.useCase = servicelevels.NewUseCase(
		s.sloRepository,
		s.manualClock.Now,
		s.eventsHandler,
	)

	scopes := concurrency.NewScopes(queueIDs, func() *servicelevels.SLOScope {
		return servicelevels.NewEmptyIntegrationsScope(s.useCase, s.sloReporter)
	})
	s.pipeline = servicelevels.NewPipeline(
		scopes,
	)
	s.eventsHandler.Pipeline = s.pipeline
}

func (s *sloPipelineSUT) NoConfigPresent(id integrations.ID, timeStr string) {
	dropErr := s.useCase.DropServiceLevels(context.Background(), id)
	Expect(dropErr).NotTo(HaveOccurred())
}

func (s *sloPipelineSUT) EmptyConfigPresent(id integrations.ID, timeStr string) {
	s.manualClock.Reset(clock.ParseTime(timeStr))
	_, err := s.useCase.ModifyRoute(
		context.Background(),
		id,
		servicelevels.RouteModification{
			Route: http.Route{
				PathPattern: "/",
			},
		})
	Expect(err).NotTo(HaveOccurred())
}

func (s *sloPipelineSUT) Report(timeStr string) []*servicelevels.CheckReport {
	now := clock.ParseTime(timeStr)
	s.pipeline.Check(&servicelevels.CheckMessage{
		Now: now,
	})

	for {
		reports := s.sloReporter.GetReports()
		if len(reports) == s.numberOfQueues {
			return reports
		}

		time.Sleep(time.Millisecond * 1)
	}
}

func (s *sloPipelineSUT) IngestOKRequest(id integrations.ID, timeStr string) {
	s.IngestOKRequestToUrl(id, timeStr, "/api/")
}

func (s *sloPipelineSUT) IngestOKRequestToUrl(id integrations.ID, timeStr string, path string) {
	now := clock.ParseTime(timeStr)
	s.pipeline.IngestHttpRequest(&servicelevels.IngestRequestsMessage{
		ID:  id,
		Now: now,
		Reqs: []*servicelevels.HttpRequest{
			{
				Latency: 1000,
				State:   "200",
				Method:  "GET",
				URL:     newUrl("https://test.com" + path),
			},
		},
	})
}

func (s *sloPipelineSUT) ChangeConfig(integrationID integrations.ID, timeStr string) {
	s.manualClock.Reset(clock.ParseTime(timeStr))

	_, err := s.useCase.ModifyRoute(context.Background(), integrationID, defaultRouteModificationForMethod("", "", "/"))
	Expect(err).NotTo(HaveOccurred())
}

func (s *sloPipelineSUT) ForDefaultConfig(integrationID integrations.ID, timeStr string) {
	s.manualClock.Reset(clock.ParseTime(timeStr))

	_, err := s.useCase.ModifyRoute(context.Background(), integrationID, defaultRouteModificationForMethod("", "", "/"))
	Expect(err).NotTo(HaveOccurred())
}

func (s *sloPipelineSUT) ForMultipleConfig(integrationID integrations.ID, timeStr string) {
	s.manualClock.Reset(clock.ParseTime(timeStr))

	_, err := s.useCase.ModifyRoute(context.Background(), integrationID, defaultRouteModificationForMethod("", "", "/"))
	Expect(err).NotTo(HaveOccurred())
	_, err = s.useCase.ModifyRoute(context.Background(), integrationID, defaultRouteModificationForMethod("", "", "/products"))
	Expect(err).NotTo(HaveOccurred())
	_, err = s.useCase.ModifyRoute(context.Background(), integrationID, defaultRouteModificationForMethod("", "", "/orders"))
	Expect(err).NotTo(HaveOccurred())
}

func (s *sloPipelineSUT) Close() {
	s.pipeline = nil
	s.sloRepository = nil
	s.sloReporter = nil
	s.manualClock = nil
	s.useCase = nil
	s.eventsHandler = nil
	s.numberOfQueues = 0
}

func (s *sloPipelineSUT) DropNonDefaultRoutes(id integrations.ID) {
	levels, getErr := s.useCase.GetServiceLevels(context.Background(), id)
	Expect(getErr).NotTo(HaveOccurred())
	for _, route := range levels.Routes {
		if route.Route.PathPattern == "/products" || route.Route.PathPattern == "/orders" {
			delErr := s.useCase.DeleteRoute(context.Background(), id, route.Key)
			Expect(delErr).NotTo(HaveOccurred())
		}
	}
}
