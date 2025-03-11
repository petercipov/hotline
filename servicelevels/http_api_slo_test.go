package servicelevels_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"hotline/servicelevels"
	"net/url"
	"time"
)

var _ = Describe("Http Api Slo", func() {

	s := suthttpapislo{}

	It("check default path service levels when no definition is set", func() {
		s.forDefaultSetup()
		s.AddRequest(&servicelevels.HttpRequest{
			Latency: 1000,
			State:   "200",
			Method:  "GET",
			URL:     newUrl("/"),
		})
		metrics := s.Check()
		Expect(len(metrics)).To(Equal(2))
		Expect(metrics[0]).To(Equal(servicelevels.SLOCheck{
			Metric: servicelevels.Metric{
				Name:  "p99",
				Value: 0,
			},
			Tags: map[string]string{
				"http_route": "/",
			},
			Breakdown: nil,
			Breach:    nil,
		}))
		Expect(metrics[1]).To(Equal(servicelevels.SLOCheck{
			Metric: servicelevels.Metric{
				Name:  "expected",
				Value: 100,
			},
			Tags: map[string]string{
				"http_route": "/",
			},
			Breakdown: []servicelevels.Metric{
				{
					Name:  "200",
					Value: 100,
				},
			},
			Breach: nil,
		}))
	})

	It("check path service levels when route defined", func() {
		s.forRouteSetup(servicelevels.HttpRouteSLODefinition{
			Path: "/users",
			Host: "iam.example.com",
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
		s.AddRequest(&servicelevels.HttpRequest{
			Latency: 1000,
			State:   "200",
			Method:  "GET",
			URL:     newUrl("https://iam.example.com/users"),
		})
		metrics := s.Check()
		Expect(len(metrics)).To(Equal(2))
		Expect(metrics[0]).To(Equal(servicelevels.SLOCheck{
			Metric: servicelevels.Metric{
				Name:  "p99",
				Value: 0,
			},
			Tags: map[string]string{
				"http_route": "iam.example.com/users",
			},
			Breakdown: nil,
			Breach:    nil,
		}))
		Expect(metrics[1]).To(Equal(servicelevels.SLOCheck{
			Metric: servicelevels.Metric{
				Name:  "expected",
				Value: 100,
			},
			Tags: map[string]string{
				"http_route": "iam.example.com/users",
			},
			Breakdown: []servicelevels.Metric{
				{
					Name:  "200",
					Value: 100,
				},
			},
			Breach: nil,
		}))
	})
})

type suthttpapislo struct {
	slo *servicelevels.HttpApiSLO
}

func (s *suthttpapislo) forDefaultSetup() {
	s.slo = servicelevels.NewHttpApiSLO(servicelevels.HttpApiSLODefinition{})
}

func (s *suthttpapislo) Check() []servicelevels.SLOCheck {
	now := parseTime("2025-02-22T12:04:55Z")
	return s.slo.Check(now)
}

func (s *suthttpapislo) AddRequest(request *servicelevels.HttpRequest) {
	now := parseTime("2025-02-22T12:04:05Z")
	s.slo.AddRequest(now, request)
}

func (s *suthttpapislo) forRouteSetup(routes ...servicelevels.HttpRouteSLODefinition) {
	s.slo = servicelevels.NewHttpApiSLO(servicelevels.HttpApiSLODefinition{
		RouteSLOs: routes,
	})
}

func newUrl(urlString string) *url.URL {
	u, err := url.Parse(urlString)
	Expect(err).To(BeNil())
	return u
}
