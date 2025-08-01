package servicelevels

import (
	"context"
	"hotline/concurrency"
	"hotline/http"
	"hotline/integrations"
	"time"
)

type SLODefinitionRepository interface {
	GetConfig(ctx context.Context, id integrations.ID) *HttpApiSLODefinition
}

type ChecksReporter interface {
	ReportChecks(ctx context.Context, report *CheckReport)
}

type SLOPipeline struct {
	fanOut *concurrency.FanOut[concurrency.ScopedAction[IntegrationsScope], IntegrationsScope]
}

func NewSLOPipeline(scopes *concurrency.Scopes[IntegrationsScope]) *SLOPipeline {
	p := &SLOPipeline{
		fanOut: concurrency.NewActionFanOut(scopes),
	}
	return p
}

func (p *SLOPipeline) IngestHttpRequest(m *IngestRequestsMessage) {
	p.fanOut.Send(m.GetMessageID(), m)
}

func (p *SLOPipeline) Check(m *CheckMessage) {
	p.fanOut.Broadcast(m)
}

func (p *SLOPipeline) ModifyRoute(m *ModifyRouteMessage) {
	p.fanOut.Send(m.GetMessageID(), m)
}

type Check struct {
	SLO           []SLOCheck
	IntegrationID integrations.ID
}

type CheckReport struct {
	Now    time.Time
	Checks []Check
}

type IntegrationsScope struct {
	Integrations     map[integrations.ID]*HttpApiSLO
	LastObservedTime time.Time

	sloRepository SLODefinitionRepository
	checkReporter ChecksReporter
}

func (scope *IntegrationsScope) AdvanceTime(now time.Time) {
	if now.After(scope.LastObservedTime) {
		scope.LastObservedTime = now
	}
}

func NewEmptyIntegrationsScope(sloRepository SLODefinitionRepository, checkReporter ChecksReporter) *IntegrationsScope {
	return &IntegrationsScope{
		Integrations:     make(map[integrations.ID]*HttpApiSLO),
		LastObservedTime: time.Time{},

		sloRepository: sloRepository,
		checkReporter: checkReporter,
	}
}

type CheckMessage struct {
	Now time.Time
}

func (message *CheckMessage) Execute(ctx context.Context, scope *IntegrationsScope) {
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

func (message *IngestRequestsMessage) GetMessageID() []byte {
	return []byte(message.ID)
}

func (message *IngestRequestsMessage) Execute(ctx context.Context, scope *IntegrationsScope) {
	scope.AdvanceTime(message.Now)

	slo, found := scope.Integrations[message.ID]
	if !found {
		config := scope.sloRepository.GetConfig(ctx, message.ID)
		if config == nil {
			return
		}
		slo = NewHttpApiSLO(*config)
		scope.Integrations[message.ID] = slo
	}
	for _, req := range message.Reqs {
		slo.AddRequest(scope.LastObservedTime, req)
	}
}

type ModifyRouteMessage struct {
	ID  integrations.ID
	Now time.Time

	Route http.Route
}

func (message *ModifyRouteMessage) Execute(ctx context.Context, scope *IntegrationsScope) {
	scope.AdvanceTime(message.Now)

	slo, found := scope.Integrations[message.ID]
	if !found {
		return
	}

	config := scope.sloRepository.GetConfig(ctx, message.ID)
	if config == nil {
		delete(scope.Integrations, message.ID)
		return
	}

	for _, slosConfig := range config.RouteSLOs {
		if slosConfig.Route == message.Route {
			slo.UpsertRoute(slosConfig)
			break
		}
	}
}

func (message *ModifyRouteMessage) GetMessageID() []byte {
	return []byte(message.ID)
}
