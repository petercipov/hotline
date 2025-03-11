package servicelevels

import (
	"fmt"
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
	pathSLOs []*HttpRouteSLO
}

type HttpApiSLODefinition struct {
	RouteSLOs []HttpRouteSLODefinition
}

type HttpRouteSLODefinition struct {
	Path    string
	Host    string
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

func NewHttpApiSLO(definition HttpApiSLODefinition) *HttpApiSLO {
	mux := http.NewServeMux()

	pathSlos := make([]*HttpRouteSLO, len(definition.RouteSLOs)+1)
	pathSlos = pathSlos[:0]
	for _, routeSLO := range definition.RouteSLOs {
		slo := NewHttpPathSLO(routeSLO)
		mux.Handle(slo.routePattern, slo)
		pathSlos = append(pathSlos, slo)
	}

	defaultSlo := NewHttpDefaultPathSLO()
	mux.Handle(defaultSlo.routePattern, defaultSlo)
	pathSlos = append(pathSlos, defaultSlo)
	return &HttpApiSLO{
		mux:      mux,
		pathSLOs: pathSlos,
	}
}

func (s *HttpApiSLO) AddRequest(now time.Time, req *HttpRequest) {
	r := &http.Request{
		Method: req.Method,
		URL:    req.URL,
		Host:   req.URL.Host,
	}

	handler, _ := s.mux.Handler(r)
	pathSLO, sloExists := handler.(*HttpRouteSLO)
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

type HttpRouteSLO struct {
	routePattern string
	stateSLO     *StateSLO
	latencySLO   *LatencySLO
}

func NewHttpPathSLO(slo HttpRouteSLODefinition) *HttpRouteSLO {
	pattern := fmt.Sprintf("%s%s", slo.Host, slo.Path)
	return &HttpRouteSLO{
		routePattern: pattern,
		stateSLO: NewStateSLO(
			slo.Status.Expected,
			[]string{},
			slo.Status.BreachThreshold,
			slo.Status.WindowDuration,
			map[string]string{
				"http_route": pattern,
			},
		),
		latencySLO: NewLatencySLO(
			slo.Latency.Percentiles,
			slo.Latency.WindowDuration,
			map[string]string{
				"http_route": pattern,
			},
		),
	}
}

func NewHttpDefaultPathSLO() *HttpRouteSLO {
	pathPattern := "/"
	return &HttpRouteSLO{
		routePattern: pathPattern,
		stateSLO: NewStateSLO(
			[]string{"200", "201"},
			[]string{},
			99.99,
			1*time.Hour,
			map[string]string{
				"http_route": pathPattern,
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
				"http_route": pathPattern,
			},
		),
	}
}

func (s *HttpRouteSLO) ServeHTTP(_ http.ResponseWriter, _ *http.Request) {
	// empty, used only for mux
}
func (s *HttpRouteSLO) AddRequest(now time.Time, req *HttpRequest) {
	s.latencySLO.AddLatency(now, req.Latency)
	s.stateSLO.AddState(now, req.State)
}

func (s *HttpRouteSLO) Check(now time.Time) []SLOCheck {
	latencyCheck := s.latencySLO.Check(now)
	stateCheck := s.stateSLO.Check(now)

	checks := make([]SLOCheck, len(latencyCheck)+len(stateCheck))
	checks = checks[:0]
	checks = append(checks, latencyCheck...)
	checks = append(checks, stateCheck...)

	return checks
}
