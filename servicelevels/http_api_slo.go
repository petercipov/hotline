package servicelevels

import (
	"errors"
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
	mux       *http.ServeMux
	routeSLOs []*HttpRouteSLO
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

func NewHttpApiSLO(definition HttpApiSLODefinition) (*HttpApiSLO, error) {
	mux := http.NewServeMux()
	routeSLOs := make([]*HttpRouteSLO, len(definition.RouteSLOs)+1)
	routeSLOs = routeSLOs[:0]
	foundDefault := false
	for _, routeSLO := range definition.RouteSLOs {
		slo := NewHttpPathSLO(routeSLO)
		if slo.routePattern == "/" {
			foundDefault = true
		}
		registerErr := safeRegisterInMux(mux, slo.routePattern, slo)
		if registerErr != nil {
			return nil, registerErr
		}
		routeSLOs = append(routeSLOs, slo)
	}
	if !foundDefault {
		return nil, errors.New("not found default route / in list of routes")
	}
	return &HttpApiSLO{
		mux:       mux,
		routeSLOs: routeSLOs,
	}, nil
}

func safeRegisterInMux(mux *http.ServeMux, pattern string, handler http.Handler) (err error) {
	defer func() {
		v := recover()
		if v != nil {
			err = fmt.Errorf("pattern %s conflicting with other route", pattern)
		}
	}()

	mux.Handle(pattern, handler)
	return nil
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
	for _, slo := range s.routeSLOs {
		check := slo.Check(now)
		checks = append(checks, check...)
	}

	return checks
}

var httpRangeBreakdown = NewHttpStateRangeBreakdown()

type HttpRouteSLO struct {
	routePattern string
	stateSLO     *StateSLO
	latencySLO   *LatencySLO
	expected     map[string]bool
}

func NewHttpPathSLO(slo HttpRouteSLODefinition) *HttpRouteSLO {
	pattern := fmt.Sprintf("%s%s", slo.Host, slo.Path)
	tags := map[string]string{
		"http_route": pattern,
	}
	expected := make(map[string]bool)
	for _, status := range slo.Status.Expected {
		expected[status] = true
	}

	return &HttpRouteSLO{
		routePattern: pattern,
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

func (s *HttpRouteSLO) ServeHTTP(_ http.ResponseWriter, _ *http.Request) {
	// empty, used only for mux
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
