package servicelevels

import (
	"net/http"
	"net/url"
	"time"
)

type HttpRequest struct {
	Latency LatencyMs
	State   string

	Method string
	URL    *url.URL
}

type HttpApiSLO struct {
	mux      *http.ServeMux
	pathSLOs []*HttpPathSLO
}

type HttpApiSLODefinition struct {
	PathsSLOs []HttpPathSLODefinition
}

type HttpPathSLODefinition struct {
	Path    string
	Latency HttpLatencySLODefinition
	Status  HttpStatusSLODefinition
}
type HttpLatencySLODefinition struct {
	Percentiles    []PercentileDefinition
	WindowDuration time.Duration
}

type HttpStatusSLODefinition struct {
	Expected        []string
	BreachThreshold Percent
	WindowDuration  time.Duration
}

func NewHttpApiSLO(_ HttpApiSLODefinition) *HttpApiSLO {
	mux := http.NewServeMux()

	pathSlo := NewHttpPathSLO("/")
	mux.Handle(pathSlo.pathPattern, pathSlo)
	return &HttpApiSLO{
		mux: mux,
		pathSLOs: []*HttpPathSLO{
			pathSlo,
		},
	}
}

func (s *HttpApiSLO) AddRequest(now time.Time, req *HttpRequest) {
	r := &http.Request{
		Method: req.Method,
		URL:    req.URL,
	}

	handler, _ := s.mux.Handler(r)
	pathSLO, sloExists := handler.(*HttpPathSLO)
	if sloExists {
		pathSLO.AddRequest(now, req)
	}

}

func (s *HttpApiSLO) Check(now time.Time) []SLOCheck {
	var checks []SLOCheck
	for _, slo := range s.pathSLOs {
		check := slo.Check(now)
		checks = append(checks, check...)
	}

	return checks
}

type HttpPathSLO struct {
	pathPattern string
	stateSLO    *StateSLO
	latencySLO  *LatencySLO
}

func NewHttpPathSLO(pathPattern string) *HttpPathSLO {
	return &HttpPathSLO{
		pathPattern: pathPattern,
		stateSLO: NewStateSLO(
			[]string{"200", "201"},
			[]string{},
			99.99,
			1*time.Hour,
			map[string]string{
				"http_path": pathPattern,
			},
		),
		latencySLO: NewLatencySLO(
			[]PercentileDefinition{
				{
					Percentile: 99.0,
					Threshold:  2000,
					Name:       "p99",
				},
			},
			1*time.Minute,
			map[string]string{
				"http_path": pathPattern,
			},
		),
	}
}

func (s *HttpPathSLO) ServeHTTP(_ http.ResponseWriter, _ *http.Request) {
	// empty, used only for mux
}
func (s *HttpPathSLO) AddRequest(now time.Time, req *HttpRequest) {
	s.latencySLO.AddLatency(now, req.Latency)
	s.stateSLO.AddState(now, req.State)
}

func (s *HttpPathSLO) Check(now time.Time) []SLOCheck {
	latencyCheck := s.latencySLO.Check(now)
	stateCheck := s.stateSLO.Check(now)

	checks := make([]SLOCheck, len(latencyCheck)+len(stateCheck))
	checks = checks[:0]
	checks = append(checks, latencyCheck...)
	checks = append(checks, stateCheck...)

	return checks
}
