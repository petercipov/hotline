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
	stateSLO   *HttpStatusSLO
	latencySLO *LatencySLO
}

func NewHttpPathSLO(slo HttpRouteSLODefinition) *HttpRouteSLO {
	tags := map[string]string{
		"http_route": slo.Route.ID(),
	}

	var stateSLO *HttpStatusSLO
	if slo.Status != nil {
		stateSLO = NewHttpStatusSLO(
			slo.Status.Expected,
			slo.Status.BreachThreshold,
			slo.Status.WindowDuration,
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
	}
}

func (s *HttpRouteSLO) AddRequest(now time.Time, req *HttpRequest) {
	if s.latencySLO != nil {
		s.latencySLO.AddLatency(now, req.Latency)
	}

	if s.stateSLO != nil {
		s.stateSLO.AddHttpState(now, req.State)
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
