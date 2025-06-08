package servicelevels_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"hotline/clock"
	"hotline/servicelevels"
	"net/url"
	"time"
)

var _ = Describe("Http Api Slo", func() {

	s := suthttpapislo{}

	It("if no default provided, return slo object", func() {
		s.forRouteSetup()
		Expect(s.slo).ShouldNot(BeNil())
	})

	It("check default path service levels when no definition is set", func() {
		s.forRouteSetupWithDefault()
		s.AddRequest(&servicelevels.HttpRequest{
			Latency: 1000,
			State:   "200",
			Method:  "GET",
			URL:     newUrl("/"),
		})
		metrics := s.Check()
		Expect(len(metrics)).To(Equal(2))
		Expect(metrics[0]).To(Equal(servicelevels.SLOCheck{
			Namespace: "http_route_latency",
			Metric: servicelevels.Metric{
				Name:        "p99",
				Value:       0,
				Unit:        "ms",
				EventsCount: 1,
			},
			Tags: map[string]string{
				"http_route": "/",
			},
			Breakdown: nil,
			Breach:    nil,
		}))
		Expect(metrics[1]).To(Equal(servicelevels.SLOCheck{
			Namespace: "http_route_status",
			Metric: servicelevels.Metric{
				Name:        "expected",
				Value:       100,
				Unit:        "%",
				EventsCount: 1,
			},
			Tags: map[string]string{
				"http_route": "/",
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
		s.forRouteSetupWithDefault(defaultRouteDefinition("iam.example.com", "/users"))
		s.AddRequest(&servicelevels.HttpRequest{
			Latency: 1000,
			State:   "200",
			Method:  "GET",
			URL:     newUrl("https://iam.example.com/users"),
		})
		metrics := s.Check()
		Expect(len(metrics)).To(Equal(2))
		Expect(metrics[0]).To(Equal(servicelevels.SLOCheck{
			Namespace: "http_route_latency",
			Metric: servicelevels.Metric{
				Name:        "p99",
				Value:       0,
				Unit:        "ms",
				EventsCount: 1,
			},
			Tags: map[string]string{
				"http_route": "iam.example.com/users",
			},
			Breakdown: nil,
			Breach:    nil,
		}))
		Expect(metrics[1]).To(Equal(servicelevels.SLOCheck{
			Namespace: "http_route_status",
			Metric: servicelevels.Metric{
				Name:        "expected",
				Value:       100,
				Unit:        "%",
				EventsCount: 1,
			},
			Tags: map[string]string{
				"http_route": "iam.example.com/users",
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

	It("duplicating default pattern will return no slo", func() {
		s.forRouteSetupWithDefault(servicelevels.HttpRouteSLODefinition{
			Path: "/",
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
		})

		Expect(s.slo).Should(BeNil())
		Expect(s.sloErr).Should(HaveOccurred())
		Expect(s.sloErr.Error()).To(Equal("pattern / conflicting with other route"))
	})

	It("breaks unexpected states by ranges", func() {
		s.forRouteSetupWithDefault(defaultRouteDefinition("iam.example.com", "/users"))
		s.AddRequest(&servicelevels.HttpRequest{
			Latency: 1000,
			State:   "404",
			Method:  "GET",
			URL:     newUrl("https://iam.example.com/users"),
		})
		s.AddRequest(&servicelevels.HttpRequest{
			Latency: 1000,
			State:   "500",
			Method:  "GET",
			URL:     newUrl("https://iam.example.com/users"),
		})
		metrics := s.Check()
		Expect(len(metrics)).To(Equal(2))
		Expect(metrics[1]).To(Equal(servicelevels.SLOCheck{
			Namespace: "http_route_status",
			Metric: servicelevels.Metric{
				Name:        "unexpected",
				Value:       100,
				Unit:        "%",
				EventsCount: 2,
			},
			Tags: map[string]string{
				"http_route": "iam.example.com/users",
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
		s.forRouteSetupWithDefault(defaultRouteDefinition("iam.example.com", "/users"))
		s.AddRequest(&servicelevels.HttpRequest{
			Latency: 1000,
			State:   "unknown_state",
			Method:  "GET",
			URL:     newUrl("https://iam.example.com/users"),
		})
		metrics := s.Check()
		Expect(len(metrics)).To(Equal(2))
		Expect(metrics[1]).To(Equal(servicelevels.SLOCheck{
			Namespace: "http_route_status",
			Metric: servicelevels.Metric{
				Name:        "unexpected",
				Value:       100,
				Unit:        "%",
				EventsCount: 1,
			},
			Tags: map[string]string{
				"http_route": "iam.example.com/users",
			},
			Breakdown: []servicelevels.Metric{
				{
					Name:        "unknown",
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
		s.forRouteSetupWithDefault(defaultRouteDefinitionForMethod("POST", "iam.example.com", "/users"))
		s.AddRequest(&servicelevels.HttpRequest{
			Latency: 1000,
			State:   "200",
			Method:  "POST",
			URL:     newUrl("https://iam.example.com/users"),
		})

		metrics := s.Check()
		Expect(len(metrics)).To(Equal(2))
		Expect(metrics[0]).To(Equal(servicelevels.SLOCheck{
			Namespace: "http_route_latency",
			Metric: servicelevels.Metric{
				Name:        "p99",
				Value:       0,
				Unit:        "ms",
				EventsCount: 1,
			},
			Tags: map[string]string{
				"http_route": "POST iam.example.com/users",
			},
			Breakdown: nil,
			Breach:    nil,
		}))
	})
})

type suthttpapislo struct {
	slo    *servicelevels.HttpApiSLO
	sloErr error
}

func (s *suthttpapislo) Check() []servicelevels.SLOCheck {
	now := clock.ParseTime("2025-02-22T12:04:55Z")
	return s.slo.Check(now)
}

func (s *suthttpapislo) AddRequest(request *servicelevels.HttpRequest) {
	now := clock.ParseTime("2025-02-22T12:04:05Z")
	s.slo.AddRequest(now, request)
}

func (s *suthttpapislo) forRouteSetup(routes ...servicelevels.HttpRouteSLODefinition) {
	s.slo, s.sloErr = servicelevels.NewHttpApiSLO(servicelevels.HttpApiSLODefinition{
		RouteSLOs: routes,
	})
}

func (s *suthttpapislo) forRouteSetupWithDefault(routes ...servicelevels.HttpRouteSLODefinition) {
	routes = append(routes, defaultRouteDefinition("", "/"))
	s.forRouteSetup(routes...)
}

func defaultRouteDefinitionForMethod(method string, host string, path string) servicelevels.HttpRouteSLODefinition {
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

func defaultRouteDefinition(host string, path string) servicelevels.HttpRouteSLODefinition {
	return defaultRouteDefinitionForMethod("", host, path)
}

func newUrl(urlString string) *url.URL {
	u, err := url.Parse(urlString)
	Expect(err).To(BeNil())
	return u
}
