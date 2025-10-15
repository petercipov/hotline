package servicelevels

import (
	"slices"
	"time"

	hotlinehttp "hotline/http"
)

type HttpRequest struct {
	Latency LatencyMs
	State   string

	Locator hotlinehttp.RequestLocator
}

type IntegrationServiceLevels struct {
	mux *hotlinehttp.Mux[HttpRouteSLO]
}

type ApiServiceLevels struct {
	Routes []RouteServiceLevels
}

func (d *ApiServiceLevels) Upsert(definition RouteServiceLevels) {
	for i, route := range d.Routes {
		if route.Route == definition.Route {
			d.Routes[i] = definition
			return
		}
	}
	d.Routes = append(d.Routes, definition)
}

func (d *ApiServiceLevels) DeleteRouteByKey(key hotlinehttp.RouteKey) (hotlinehttp.Route, bool) {
	for i, route := range d.Routes {
		if route.Key == key {
			d.Routes = append(d.Routes[:i], d.Routes[i+1:]...)
			return route.Route, true
		}
	}
	return hotlinehttp.Route{}, false
}

type RouteServiceLevels struct {
	Route      hotlinehttp.Route
	Key        hotlinehttp.RouteKey
	Latency    *LatencyServiceLevels
	Status     *StatusServiceLevels
	Validation *ValidationServiceLevels
	CreatedAt  time.Time
}

type LatencyServiceLevels struct {
	Percentiles    []PercentileDefinition
	WindowDuration time.Duration
}

type StatusServiceLevels struct {
	Expected        []string
	BreachThreshold Percentile
	WindowDuration  time.Duration
}

type ValidationServiceLevels struct {
	BreachThreshold Percentile
	WindowDuration  time.Duration
}

func NewHttpApiServiceLevels(definition ApiServiceLevels) *IntegrationServiceLevels {
	apiSlo := &IntegrationServiceLevels{
		mux: &hotlinehttp.Mux[HttpRouteSLO]{},
	}
	for _, routeDefinition := range definition.Routes {
		apiSlo.UpsertRoute(routeDefinition)
	}
	return apiSlo
}

func (s *IntegrationServiceLevels) AddRequest(now time.Time, req *HttpRequest) {
	handler := s.mux.LocaleHandler(req.Locator)
	if handler != nil {
		handler.AddRequest(now, req)
	}
}

func (s *IntegrationServiceLevels) Check(now time.Time) []LevelsCheck {
	var checks []LevelsCheck
	for _, slo := range s.mux.Handlers() {
		check := slo.Check(now)
		checks = append(checks, check...)
	}

	return checks
}

func (s *IntegrationServiceLevels) UpsertRoute(routeDefinition RouteServiceLevels) {
	slo := NewHttpPathSLO(routeDefinition)
	s.mux.Upsert(slo.route, slo)
}

func (s *IntegrationServiceLevels) DeleteRoute(route hotlinehttp.Route) {
	s.mux.Delete(route)
}

func (s *IntegrationServiceLevels) AddRequestValidation(now time.Time, locator hotlinehttp.RequestLocator, status ValidationStatus) {
	handler := s.mux.LocaleHandler(locator)
	if handler != nil {
		handler.AddRequestValidation(now, status)
	}
}

type HttpRouteSLO struct {
	route         hotlinehttp.Route
	stateSLO      *HttpStatusSLO
	latencySLO    *LatencySLO
	validationSLO *ValidationSLO
}

func NewHttpPathSLO(slo RouteServiceLevels) *HttpRouteSLO {
	tags := map[string]string{
		"http_route": slo.Key.String(),
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

	var validationSLO *ValidationSLO
	if slo.Validation != nil {
		validationSLO = NewValidationSLO(
			slo.Validation.BreachThreshold,
			slo.Validation.WindowDuration,
			"http_route_validation",
			tags,
			slo.CreatedAt,
		)
	}

	return &HttpRouteSLO{
		route:         slo.Route,
		stateSLO:      stateSLO,
		latencySLO:    latencySLO,
		validationSLO: validationSLO,
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

func (s *HttpRouteSLO) Check(now time.Time) []LevelsCheck {
	var latencyCheck []LevelsCheck
	if s.latencySLO != nil {
		latencyCheck = s.latencySLO.Check(now)
	}

	var stateCheck []LevelsCheck
	if s.stateSLO != nil {
		stateCheck = s.stateSLO.Check(now)
	}

	var validationCheck []LevelsCheck
	if s.validationSLO != nil {
		validationCheck = s.validationSLO.Check(now)
	}

	return slices.Concat(latencyCheck, stateCheck, validationCheck)
}

func (s *HttpRouteSLO) AddRequestValidation(now time.Time, status ValidationStatus) {
	if s.validationSLO != nil {
		s.validationSLO.AddValidation(now, status)
	}
}
