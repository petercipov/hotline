package servicelevels

import (
	"net/url"
	"slices"
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

func (d *HttpApiSLODefinition) DeleteRouteByKey(key string) (hotlinehttp.Route, bool) {
	for i, route := range d.Routes {
		if route.Route.ID() == key {
			d.Routes = append(d.Routes[:i], d.Routes[i+1:]...)
			return route.Route, true
		}
	}
	return hotlinehttp.Route{}, false
}

type HttpRouteSLODefinition struct {
	Route   hotlinehttp.Route
	Latency *HttpLatencySLODefinition
	Status  *HttpStatusSLODefinition
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

func (s *HttpApiSLO) DeleteRoute(route hotlinehttp.Route) {
	s.mux.Delete(route)
}

type HttpRouteSLO struct {
	route      hotlinehttp.Route
	stateSLO   *StateSLO
	latencySLO *LatencySLO
	expected   map[string]bool
	breakdown  *HttpStateRangeBreakdown
}

func NewHttpPathSLO(slo HttpRouteSLODefinition) *HttpRouteSLO {
	tags := map[string]string{
		"http_route": slo.Route.ID(),
	}

	breakdown := NewHttpStateRangeBreakdown()

	expected := make(map[string]bool)
	var stateSLO *StateSLO
	if slo.Status != nil {
		for _, status := range slo.Status.Expected {
			expected[status] = true
		}
		stateSLO = NewStateSLO(
			slo.Status.Expected,
			breakdown.GetRanges(),
			slo.Status.BreachThreshold,
			slo.Status.WindowDuration,
			"http_route_status",
			tags,
		)
	}

	var latencySLO *LatencySLO
	if slo.Latency != nil {
		latencySLO = NewLatencySLO(
			slo.Latency.Percentiles,
			slo.Latency.WindowDuration,
			"http_route_latency",
			tags,
		)
	}

	return &HttpRouteSLO{
		route:      slo.Route,
		stateSLO:   stateSLO,
		latencySLO: latencySLO,
		expected:   expected,
		breakdown:  breakdown,
	}
}

func (s *HttpRouteSLO) AddRequest(now time.Time, req *HttpRequest) {
	if s.latencySLO != nil {
		s.latencySLO.AddLatency(now, req.Latency)
	}

	if s.stateSLO != nil {
		_, isExpected := s.expected[req.State]
		if isExpected {
			s.stateSLO.AddState(now, req.State)
			return
		}

		httpRange := s.breakdown.GetRange(req.State)
		if httpRange != nil {
			s.stateSLO.AddState(now, *httpRange)
			return
		}

		s.stateSLO.AddState(now, "unknown")
	}
}

func (s *HttpRouteSLO) Check(now time.Time) []SLOCheck {
	var latencyCheck []SLOCheck
	if s.latencySLO != nil {
		latencyCheck = s.latencySLO.Check(now)
	}

	var stateCheck []SLOCheck
	if s.stateSLO != nil {
		stateCheck = s.stateSLO.Check(now)
	}

	return slices.Concat(latencyCheck, stateCheck)
}
