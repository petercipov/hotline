package servicelevels

import (
	"net/url"
	"time"

	hotlinehttp "hotline/http"
)

type HttpRequest struct {
	Latency LatencyMs
	State   string

	Method string
	URL    *url.URL
}

type HttpApiSLO struct {
	mux       *hotlinehttp.Mux[HttpRouteSLO]
	routeSLOs []*HttpRouteSLO
}

type HttpApiSLODefinition struct {
	RouteSLOs []HttpRouteSLODefinition
}

type HttpRouteSLODefinition struct {
	Route   hotlinehttp.Route
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
	mux := &hotlinehttp.Mux[HttpRouteSLO]{}
	routeSLOs := make([]*HttpRouteSLO, len(definition.RouteSLOs)+1)
	routeSLOs = routeSLOs[:0]
	for _, routeSLO := range definition.RouteSLOs {
		slo := NewHttpPathSLO(routeSLO)
		mux.Add(slo.route, slo)
		routeSLOs = append(routeSLOs, slo)
	}
	return &HttpApiSLO{
		mux:       mux,
		routeSLOs: routeSLOs,
	}
}

func (s *HttpApiSLO) AddRequest(now time.Time, req *HttpRequest) {
	locator := hotlinehttp.RequestLocator{
		Method: req.Method,
		Path:   req.URL.Path,
		Host:   req.URL.Hostname(),
		Port:   80,
	}

	handler := s.mux.LocaleHandler(locator)
	if handler != nil {
		handler.AddRequest(now, req)
	}
}

func (s *HttpApiSLO) Check(now time.Time) []SLOCheck {
	var checks []SLOCheck
	for _, slo := range s.routeSLOs {
		check := slo.Check(now)
		checks = append(checks, check...)
	}

	return checks
}

var httpRangeBreakdown = NewHttpStateRangeBreakdown()

type HttpRouteSLO struct {
	route      hotlinehttp.Route
	stateSLO   *StateSLO
	latencySLO *LatencySLO
	expected   map[string]bool
}

func NewHttpPathSLO(slo HttpRouteSLODefinition) *HttpRouteSLO {
	tags := map[string]string{
		"http_route": slo.Route.ID(),
	}
	expected := make(map[string]bool)
	for _, status := range slo.Status.Expected {
		expected[status] = true
	}

	return &HttpRouteSLO{
		route: slo.Route,
		stateSLO: NewStateSLO(
			slo.Status.Expected,
			httpRangeBreakdown.GetRanges(),
			slo.Status.BreachThreshold,
			slo.Status.WindowDuration,
			"http_route_status",
			tags,
		),
		latencySLO: NewLatencySLO(
			slo.Latency.Percentiles,
			slo.Latency.WindowDuration,
			"http_route_latency",
			tags,
		),
		expected: expected,
	}
}

func (s *HttpRouteSLO) AddRequest(now time.Time, req *HttpRequest) {
	s.latencySLO.AddLatency(now, req.Latency)

	_, isExpected := s.expected[req.State]
	if isExpected {
		s.stateSLO.AddState(now, req.State)
		return
	}

	httpRange := httpRangeBreakdown.GetRange(req.State)
	if httpRange != nil {
		s.stateSLO.AddState(now, *httpRange)
		return
	}

	s.stateSLO.AddState(now, "unknown")
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
