package servicelevels_test

import (
	"hotline/clock"
	"hotline/http"
	"hotline/servicelevels"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Service Levels Checker", func() {

	s := suthttpapislo{}

	It("if no default provided, return slo object", func() {
		s.forRouteSetup()
		Expect(s.slo).ShouldNot(BeNil())
	})

	It("if no service levels defined, no checks are done", func() {
		s.forRouteSetupWithDefault(servicelevels.RouteModification{
			Route: http.Route{
				Method:      "GET",
				Host:        "iam.example.com",
				PathPattern: "/users",
			},
		})
		s.AddRequest(&servicelevels.HttpRequest{
			Latency: 1000,
			State:   "200",
			Locator: http.RequestLocator{
				Method: "GET",
				Path:   "/users",
				Host:   "iam.example.com",
				Port:   443,
			},
		})
		metrics := s.Check()
		Expect(metrics).To(BeEmpty())
	})

	It("check default path service levels when no definition is set", func() {
		s.forRouteSetupWithDefault()
		s.AddRequest(&servicelevels.HttpRequest{
			Latency: 1000,
			State:   "200",
			Locator: http.RequestLocator{
				Method: "GET",
				Path:   "/",
			},
		})
		metrics := s.Check()
		Expect(metrics).To(HaveLen(2))
		Expect(metrics[0]).To(Equal(servicelevels.LevelsCheck{
			Namespace: "http_route_latency",
			Timestamp: clock.ParseTime("2025-02-22T12:02:10.001Z"),
			Uptime:    1 * time.Millisecond,
			Metric: servicelevels.Metric{
				Name:        "p99",
				Value:       0,
				Unit:        "ms",
				EventsCount: 1,
			},
			Tags: map[string]string{
				"http_route": "RKumkv7ml0-xQ",
			},
			Breakdown: nil,
			Breach:    nil,
		}))
		Expect(metrics[1]).To(Equal(servicelevels.LevelsCheck{
			Namespace: "http_route_status",
			Timestamp: clock.ParseTime("2025-02-22T12:02:10.001Z"),
			Uptime:    1 * time.Millisecond,
			Metric: servicelevels.Metric{
				Name:        "expected",
				Value:       100,
				Unit:        "%",
				EventsCount: 1,
			},
			Tags: map[string]string{
				"http_route": "RKumkv7ml0-xQ",
			},
			Breakdown: []servicelevels.Metric{
				{
					Name:        "200",
					Value:       100,
					Unit:        "%",
					EventsCount: 1,
				},
			},
			Breach: nil,
		}))
	})

	It("check route service levels when route defined", func() {
		s.forRouteSetupWithDefault(defaultRouteModificationForMethod("", "iam.example.com", "/users"))
		s.AddRequest(&servicelevels.HttpRequest{
			Latency: 1000,
			State:   "200",
			Locator: http.RequestLocator{
				Method: "GET",
				Path:   "/users",
				Host:   "iam.example.com",
				Port:   443,
			},
		})
		metrics := s.Check()
		Expect(metrics).To(HaveLen(2))
		Expect(metrics[0]).To(Equal(servicelevels.LevelsCheck{
			Namespace: "http_route_latency",
			Timestamp: clock.ParseTime("2025-02-22T12:02:10.001Z"),
			Uptime:    1 * time.Millisecond,
			Metric: servicelevels.Metric{
				Name:        "p99",
				Value:       0,
				Unit:        "ms",
				EventsCount: 1,
			},
			Tags: map[string]string{
				"http_route": "RKjhq92VEGBI4",
			},
			Breakdown: nil,
			Breach:    nil,
		}))
		Expect(metrics[1]).To(Equal(servicelevels.LevelsCheck{
			Namespace: "http_route_status",
			Timestamp: clock.ParseTime("2025-02-22T12:02:10.001Z"),
			Uptime:    1 * time.Millisecond,
			Metric: servicelevels.Metric{
				Name:        "expected",
				Value:       100,
				Unit:        "%",
				EventsCount: 1,
			},
			Tags: map[string]string{
				"http_route": "RKjhq92VEGBI4",
			},
			Breakdown: []servicelevels.Metric{
				{
					Name:        "200",
					Value:       100,
					Unit:        "%",
					EventsCount: 1,
				},
			},
			Breach: nil,
		}))
	})

	It("breaks unexpected states by ranges", func() {
		s.forRouteSetupWithDefault(defaultRouteModificationForMethod("", "iam.example.com", "/users"))
		s.AddRequest(&servicelevels.HttpRequest{
			Latency: 1000,
			State:   "404",
			Locator: http.RequestLocator{
				Method: "GET",
				Path:   "/users",
				Host:   "iam.example.com",
				Port:   443,
			},
		})
		s.AddRequest(&servicelevels.HttpRequest{
			Latency: 1000,
			State:   "500",
			Locator: http.RequestLocator{
				Method: "GET",
				Path:   "/users",
				Host:   "iam.example.com",
				Port:   443,
			},
		})
		metrics := s.Check()
		Expect(metrics).To(HaveLen(2))
		Expect(metrics[1]).To(Equal(servicelevels.LevelsCheck{
			Namespace: "http_route_status",
			Timestamp: clock.ParseTime("2025-02-22T12:02:10.0015Z"),
			Uptime:    1*time.Millisecond + 500*time.Microsecond,
			Metric: servicelevels.Metric{
				Name:        "unexpected",
				Value:       100,
				Unit:        "%",
				EventsCount: 2,
			},
			Tags: map[string]string{
				"http_route": "RKjhq92VEGBI4",
			},
			Breakdown: []servicelevels.Metric{
				{
					Name:        "4xx",
					Value:       50,
					Unit:        "%",
					EventsCount: 1,
				},
				{
					Name:        "5xx",
					Value:       50,
					Unit:        "%",
					EventsCount: 1,
				},
			},
			Breach: &servicelevels.SLOBreach{
				ThresholdValue: 0.1,
				ThresholdUnit:  "%",
				Operation:      "<",
				WindowDuration: 1 * time.Hour,
			},
		}))
	})

	It("breaks unknown unexpected state", func() {
		s.forRouteSetupWithDefault(defaultRouteModificationForMethod("", "iam.example.com", "/users"))
		s.AddRequest(&servicelevels.HttpRequest{
			Latency: 1000,
			State:   "unknown_state",
			Locator: http.RequestLocator{
				Method: "GET",
				Path:   "/users",
				Host:   "iam.example.com",
				Port:   443,
			},
		})
		metrics := s.Check()
		Expect(metrics).To(HaveLen(2))
		Expect(metrics[1]).To(Equal(servicelevels.LevelsCheck{
			Namespace: "http_route_status",
			Timestamp: clock.ParseTime("2025-02-22T12:02:10.001Z"),
			Uptime:    1 * time.Millisecond,
			Metric: servicelevels.Metric{
				Name:        "unexpected",
				Value:       100,
				Unit:        "%",
				EventsCount: 1,
			},
			Tags: map[string]string{
				"http_route": "RKjhq92VEGBI4",
			},
			Breakdown: []servicelevels.Metric{
				{
					Name:        "range_unknown",
					Value:       100,
					Unit:        "%",
					EventsCount: 1,
				},
			},
			Breach: &servicelevels.SLOBreach{
				ThresholdValue: 0.1,
				ThresholdUnit:  "%",
				Operation:      "<",
				WindowDuration: 1 * time.Hour,
			},
		}))
	})

	It("creates pattern per http method", func() {
		s.forRouteSetupWithDefault(defaultRouteModificationForMethod("POST", "iam.example.com", "/users"))
		s.AddRequest(&servicelevels.HttpRequest{
			Latency: 1000,
			State:   "200",
			Locator: http.RequestLocator{
				Method: "POST",
				Path:   "/users",
				Host:   "iam.example.com",
				Port:   443,
			},
		})

		metrics := s.Check()
		Expect(metrics).To(HaveLen(2))
		Expect(metrics[0]).To(Equal(servicelevels.LevelsCheck{
			Namespace: "http_route_latency",
			Timestamp: clock.ParseTime("2025-02-22T12:02:10.001Z"),
			Uptime:    1 * time.Millisecond,
			Metric: servicelevels.Metric{
				Name:        "p99",
				Value:       0,
				Unit:        "ms",
				EventsCount: 1,
			},
			Tags: map[string]string{
				"http_route": "RKROuLp_UwI5A",
			},
			Breakdown: nil,
			Breach:    nil,
		}))
	})

	It("will override default route definition", func() {
		def := defaultRouteModification("iam.example.com", "/users")
		def.Latency.WindowDuration = 1 * time.Minute

		sloDef := servicelevels.ApiServiceLevels{}

		sloDef.Upsert(def)
		Expect(sloDef.Routes[0].Latency.WindowDuration).To(Equal(1 * time.Minute))

		defNext := defaultRouteModification("iam.example.com", "/users")
		defNext.Latency.WindowDuration = 10 * time.Minute
		sloDef.Upsert(defNext)

		Expect(sloDef.Routes).To(HaveLen(1))
		Expect(sloDef.Routes[0].Latency.WindowDuration).To(Equal(10 * time.Minute))

	})

	It("will delete default route definition", func() {
		def := defaultRouteModification("iam.example.com", "/users")
		sloDef := servicelevels.ApiServiceLevels{}
		sloDef.Upsert(def)
		Expect(sloDef.Routes).To(HaveLen(1))

		sloDef.DeleteRouteByKey("RKjhq92VEGBI4")

		Expect(sloDef.Routes).To(BeEmpty())
	})

	It("will not delete for unknown key", func() {
		def := defaultRouteModification("iam.example.com", "/users")
		sloDef := servicelevels.ApiServiceLevels{}
		sloDef.Upsert(def)
		Expect(sloDef.Routes).To(HaveLen(1))
		sloDef.DeleteRouteByKey(":uknown::")
		Expect(sloDef.Routes).To(HaveLen(1))
	})
})

type suthttpapislo struct {
	slo        *servicelevels.IntegrationServiceLevels
	manulClock *clock.ManualClock
}

func (s *suthttpapislo) Check() []servicelevels.LevelsCheck {
	return s.slo.Check(s.manulClock.Now())
}

func (s *suthttpapislo) AddRequest(request *servicelevels.HttpRequest) {
	s.slo.AddRequest(s.manulClock.Now(), request)
}

func (s *suthttpapislo) forRouteSetup(routes ...servicelevels.RouteModification) {
	definition := servicelevels.ApiServiceLevels{}

	for _, route := range routes {
		definition.Upsert(servicelevels.RouteServiceLevels{
			Route:   route.Route,
			Key:     route.Route.GenerateKey("integration-id"),
			Latency: route.Latency,
			Status:  route.Status,
		})
	}
	s.manulClock = clock.NewDefaultManualClock()
	s.slo = servicelevels.NewHttpApiServiceLevels(definition, s.manulClock.Now())
}

func (s *suthttpapislo) forRouteSetupWithDefault(routes ...servicelevels.RouteModification) {
	routes = append(routes, defaultRouteModificationForMethod("", "", "/"))
	s.forRouteSetup(routes...)
}

func defaultRouteModificationForMethod(method string, host string, pathPattern string) servicelevels.RouteModification {
	route := http.Route{
		Method:      method,
		PathPattern: pathPattern,
		Host:        host,
	}
	return servicelevels.RouteModification{
		Route: route,
		Latency: &servicelevels.LatencyServiceLevels{
			Percentiles: []servicelevels.PercentileDefinition{
				{
					Percentile: 0.999,
					Threshold:  2000,
					Name:       "p99",
				},
			},
			WindowDuration: 1 * time.Minute,
		},
		Status: &servicelevels.StatusServiceLevels{
			Expected:        []string{"200", "201"},
			BreachThreshold: 0.999,
			WindowDuration:  1 * time.Hour,
		},
	}
}

func defaultRouteModification(host string, path string) servicelevels.RouteServiceLevels {
	modification := defaultRouteModificationForMethod("", host, path)
	return servicelevels.RouteServiceLevels{
		Route:   modification.Route,
		Key:     modification.Route.GenerateKey("integration-id"),
		Latency: modification.Latency,
		Status:  modification.Status,
	}
}
