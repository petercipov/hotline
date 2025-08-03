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
	mux *hotlinehttp.Mux[HttpRouteSLO]
}

type HttpApiSLODefinition struct {
	Routes []HttpRouteSLODefinition
}

func (d *HttpApiSLODefinition) Upsert(definition HttpRouteSLODefinition) {
	for i, route := range d.Routes {
		if route.Route == definition.Route {
			d.Routes[i] = definition
			return
		}
	}
	d.Routes = append(d.Routes, definition)
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
	BreachThreshold Percentile
	WindowDuration  time.Duration
}

func NewHttpApiSLO(definition HttpApiSLODefinition) *HttpApiSLO {
	apiSlo := &HttpApiSLO{
		mux: &hotlinehttp.Mux[HttpRouteSLO]{},
	}
	for _, routeDefinition := range definition.Routes {
		apiSlo.UpsertRoute(routeDefinition)
	}
	return apiSlo
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
	for _, slo := range s.mux.Handlers() {
		check := slo.Check(now)
		checks = append(checks, check...)
	}

	return checks
}

func (s *HttpApiSLO) UpsertRoute(routeDefinition HttpRouteSLODefinition) {
	slo := NewHttpPathSLO(routeDefinition)
	s.mux.Upsert(slo.route, slo)
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
