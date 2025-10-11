package servicelevels

import (
	"context"
	"errors"
	"hotline/concurrency"
	"hotline/http"
	"hotline/integrations"
	"time"
)

var ErrServiceLevelsNotFound = errors.New("service levels not found")
var ErrRouteNotFound = errors.New("route not found")

type Reader interface {
	GetServiceLevels(ctx context.Context, id integrations.ID) (ApiServiceLevels, error)
}

type ChecksReporter interface {
	ReportChecks(ctx context.Context, report *CheckReport)
}

type Pipeline struct {
	fanOut *concurrency.FanOut[concurrency.ScopedAction[SLOScope], SLOScope]
}

func NewPipeline(scopes *concurrency.Scopes[SLOScope]) *Pipeline {
	p := &Pipeline{
		fanOut: concurrency.NewActionFanOut(scopes),
	}
	return p
}

func (p *Pipeline) IngestHttpRequest(m *IngestRequestsMessage) {
	p.fanOut.Send(m.GetShardingKey(), m)
}

func (p *Pipeline) Check(m *CheckMessage) {
	p.fanOut.Broadcast(m)
}

func (p *Pipeline) RouteModified(m *ModifyForRouteMessage) {
	p.fanOut.Send(m.GetShardingKey(), m)
}

type Check struct {
	SLO           []SLOCheck
	IntegrationID integrations.ID
}

type CheckReport struct {
	Now    time.Time
	Checks []Check
}

type SLOScope struct {
	Integrations     map[integrations.ID]*Checker
	LastObservedTime time.Time

	sloRepository Reader
	checkReporter ChecksReporter
}

func (scope *SLOScope) AdvanceTime(now time.Time) {
	if now.After(scope.LastObservedTime) {
		scope.LastObservedTime = now
	}
}

func NewEmptyIntegrationsScope(sloRepository Reader, checkReporter ChecksReporter) *SLOScope {
	return &SLOScope{
		Integrations:     make(map[integrations.ID]*Checker),
		LastObservedTime: time.Time{},

		sloRepository: sloRepository,
		checkReporter: checkReporter,
	}
}

type CheckMessage struct {
	Now time.Time
}

func (message *CheckMessage) Execute(ctx context.Context, _ string, scope *SLOScope) {
	scope.AdvanceTime(message.Now)

	var checks []Check
	for id, integration := range scope.Integrations {
		metrics := integration.Check(scope.LastObservedTime)
		checks = append(checks, Check{
			SLO:           metrics,
			IntegrationID: id,
		})
	}

	scope.checkReporter.ReportChecks(ctx, &CheckReport{
		Now:    scope.LastObservedTime,
		Checks: checks,
	})
}

type IngestRequestsMessage struct {
	ID  integrations.ID
	Now time.Time

	Reqs []*HttpRequest
}

func (message *IngestRequestsMessage) GetShardingKey() []byte {
	return []byte(message.ID)
}

func (message *IngestRequestsMessage) Execute(ctx context.Context, _ string, scope *SLOScope) {
	scope.AdvanceTime(message.Now)

	slo, found := scope.Integrations[message.ID]
	if !found {
		config, getErr := scope.sloRepository.GetServiceLevels(ctx, message.ID)
		if getErr != nil {
			return
		}
		slo = NewHttpApiServiceLevels(config)
		scope.Integrations[message.ID] = slo
	}
	for _, req := range message.Reqs {
		slo.AddRequest(scope.LastObservedTime, req)
	}
}

type ModifyForRouteMessage struct {
	ID  integrations.ID
	Now time.Time

	Route http.Route
}

func (message *ModifyForRouteMessage) Execute(ctx context.Context, _ string, scope *SLOScope) {
	scope.AdvanceTime(message.Now)

	slo, found := scope.Integrations[message.ID]
	if !found {
		return
	}
	config, getErr := scope.sloRepository.GetServiceLevels(ctx, message.ID)
	if getErr != nil {
		if errors.Is(getErr, ErrServiceLevelsNotFound) {
			delete(scope.Integrations, message.ID)
		}
		return
	}

	var foundRouteConfig = false
	var routeConfig RouteServiceLevels
	for _, slosConfig := range config.Routes {
		if slosConfig.Route == message.Route {
			foundRouteConfig = true
			routeConfig = slosConfig
			break
		}
	}
	if foundRouteConfig {
		slo.UpsertRoute(routeConfig)
	} else {
		slo.DeleteRoute(message.Route)
	}
}

func (message *ModifyForRouteMessage) GetShardingKey() []byte {
	return []byte(message.ID)
}

type EventsHandler struct {
	Pipeline *Pipeline
}

func (h *EventsHandler) HandleRouteModified(messages []ModifyForRouteMessage) error {
	for _, m := range messages {
		h.Pipeline.RouteModified(&m)
	}
	return nil
}
