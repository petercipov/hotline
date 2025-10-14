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
			sut.NoConfigPresentForIntegration(id)
			sut.IngestOKRequestForIntegration(id)
		}
		reports := sut.Report()
		Expect(reports).To(HaveLen(sut.numberOfQueues))
		for _, report := range reports {
			Expect(report).To(BeEmpty())
		}
	})

	It("should report metrics if configuration available", func() {
		sut.forPipeline()
		sut.ForDefaultConfig()
		for range 1000 {
			sut.IngestOKRequest()
		}
		reports := sut.Report()
		Expect(reports).To(HaveLen(sut.numberOfQueues))

		var nonEmptyReports []servicelevels.CheckReport
		for _, report := range reports {
			if len(report) > 0 {
				nonEmptyReports = append(nonEmptyReports, report)
			}
		}
		Expect(nonEmptyReports).To(HaveLen(1))
	})

	Context("updating slo config", func() {
		It("should report different config if route was changed", func() {
			sut.forPipeline()
			sut.ForDefaultConfig()
			for range 1000 {
				sut.IngestOKRequest()
			}

			sut.ChangeConfig()

			reports := sut.Report()
			Expect(reports).To(HaveLen(sut.numberOfQueues))

			var nonEmptyReports []servicelevels.CheckReport
			for _, report := range reports {
				if len(report) > 0 {
					nonEmptyReports = append(nonEmptyReports, report)
				}
			}
			Expect(nonEmptyReports).To(HaveLen(1))
		})

		It("should report nothing when config was removed", func() {
			sut.forPipeline()
			sut.ForDefaultConfig()
			for range 10 {
				sut.IngestOKRequest()
			}
			sut.NoConfigPresent()

			reports := sut.Report()
			Expect(reports).To(HaveLen(sut.numberOfQueues))

			for _, report := range reports {
				Expect(report).To(BeEmpty())
			}
		})

		It("should report nothing when config was emptied", func() {
			sut.forPipeline()
			sut.ForDefaultConfig()
			for range 10 {
				sut.IngestOKRequest()
			}
			sut.EmptyConfigPresent()

			reports := sut.Report()
			Expect(reports).To(HaveLen(sut.numberOfQueues))

			for _, report := range reports {
				if len(report) == 1 {
					Expect(report[0].Levels).To(BeEmpty())
					Expect(string(report[0].IntegrationID)).To(Equal("known_integration_id"))
				} else {
					Expect(report).To(BeEmpty())
				}
			}
		})
	})

	It("should report less when config is reduced from multiple to single slo", func() {
		sut.forPipeline()
		sut.ForMultipleConfig()

		sut.IngestOKRequestForPath("/api")
		sut.IngestOKRequestForPath("/products")
		sut.IngestOKRequestForPath("/orders")

		sut.DropNonDefaultRoutes("known_integration_id")

		reports := sut.Report()
		Expect(reports).To(HaveLen(sut.numberOfQueues))

		checks := 0
		for _, report := range reports {
			checks += len(report)
		}
		Expect(checks).To(Equal(1))
	})

	Context("request validation", func() {

		It("computes nothing if service levels are not configured", func() {
			sut.forPipeline()
			sut.IngestValidationMessage()

			reports := sut.Report()
			Expect(reports).To(HaveLen(sut.numberOfQueues))
			byIntegration := reports.GroupByIntegrationID()
			Expect(byIntegration).To(BeEmpty())
		})

		It("computes number of not validated requests, when no validation was done", func() {
			sut.forPipeline()
			sut.forConfigWithRequestValidation()

			for range 10 {
				sut.IngestValidationMessage()
			}
			reports := sut.Report()
			Expect(reports).To(HaveLen(sut.numberOfQueues))
			byIntegration := reports.GroupByIntegrationID()
			Expect(byIntegration).NotTo(BeEmpty())
			integrationChecks := byIntegration["known_integration_id"]
			Expect(integrationChecks).To(HaveLen(1))
			Expect(integrationChecks[0]).To(Equal(servicelevels.LevelsCheck{
				Namespace: "http_route_validation",
				Timestamp: clock.ParseTime("2025-02-22T12:02:10.0055Z"),
				Metric: servicelevels.Metric{
					Name:        "skipped",
					Value:       100,
					Unit:        "%",
					EventsCount: 10,
				},
				Tags: map[string]string{
					"http_route": "RKpMj21xeTHEQ",
				},
			}))
		})
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
	s.manualClock = clock.NewDefaultManualClock()
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

func (s *sloPipelineSUT) NoConfigPresentForIntegration(id integrations.ID) {
	dropErr := s.useCase.DropServiceLevels(context.Background(), id)
	Expect(dropErr).NotTo(HaveOccurred())
}

func (s *sloPipelineSUT) NoConfigPresent() {
	s.NoConfigPresentForIntegration("known_integration_id")
}

func (s *sloPipelineSUT) EmptyConfigPresent() {
	_, err := s.useCase.ModifyRoute(
		context.Background(),
		"known_integration_id",
		servicelevels.RouteModification{
			Route: http.Route{
				PathPattern: "/",
			},
		})
	Expect(err).NotTo(HaveOccurred())
}

func (s *sloPipelineSUT) Report() servicelevels.ReportArr {
	now := s.manualClock.Now()
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

func (s *sloPipelineSUT) IngestOKRequestForIntegration(id integrations.ID) {
	s.IngestOKRequestForIntegrationAndPath(id, "/api/")
}

func (s *sloPipelineSUT) IngestOKRequest() {
	s.IngestOKRequestForIntegrationAndPath("known_integration_id", "/api/")
}

func (s *sloPipelineSUT) IngestOKRequestForPath(path string) {
	s.IngestOKRequestForIntegrationAndPath("known_integration_id", path)
}

func (s *sloPipelineSUT) IngestOKRequestForIntegrationAndPath(id integrations.ID, path string) {
	now := s.manualClock.Now()
	s.pipeline.IngestHttpRequest(&servicelevels.IngestRequestsMessage{
		ID:  id,
		Now: now,
		Reqs: []*servicelevels.HttpRequest{
			{
				Latency: 1000,
				State:   "200",
				Locator: http.RequestLocator{
					Method: "GET",
					Path:   path,
					Host:   "test.com",
					Port:   443,
				},
			},
		},
	})
}

func (s *sloPipelineSUT) ChangeConfig() {
	_, err := s.useCase.ModifyRoute(
		context.Background(),
		"known_integration_id",
		defaultRouteModificationForMethod("", "", "/"))
	Expect(err).NotTo(HaveOccurred())
}

func (s *sloPipelineSUT) ForDefaultConfigForIntegration(integrationID integrations.ID) {
	_, err := s.useCase.ModifyRoute(context.Background(), integrationID, defaultRouteModificationForMethod("", "", "/"))
	Expect(err).NotTo(HaveOccurred())
}

func (s *sloPipelineSUT) ForDefaultConfig() {
	s.ForDefaultConfigForIntegration("known_integration_id")
}

func (s *sloPipelineSUT) ForMultipleConfig() {
	integrationID := integrations.ID("known_integration_id")
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

func (s *sloPipelineSUT) forConfigWithRequestValidation() {
	_, err := s.useCase.ModifyRoute(
		context.Background(),
		"known_integration_id",
		servicelevels.RouteModification{
			Route: http.Route{
				Method:      "GET",
				PathPattern: "/products",
			},
			Validation: &servicelevels.ValidationServiceLevels{},
		},
	)
	Expect(err).NotTo(HaveOccurred())
}

func (s *sloPipelineSUT) IngestValidationMessage() {
	s.pipeline.RequestValidated(&servicelevels.RequestValidatedMessage{
		ID:  "known_integration_id",
		Now: s.manualClock.Now(),
		Locator: http.RequestLocator{
			Method: "GET",
			Path:   "/products",
			Host:   "test.com",
			Port:   443,
		},
	})
}
