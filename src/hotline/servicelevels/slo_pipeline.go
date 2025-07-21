package servicelevels

import (
	"context"
	"hotline/concurrency"
	"hotline/integrations"
	"time"
)

type IntegrationSLORepository interface {
	GetIntegrationSLO(ctx context.Context, id integrations.ID) *IntegrationSLO
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

func (p *SLOPipeline) IngestHttpRequests(messages ...*HttpReqsMessage) {
	for _, m := range messages {
		p.fanOut.Send(m.GetMessageID(), m)
	}
}

func (p *SLOPipeline) Check(m *CheckMessage) {
	p.fanOut.Broadcast(m)
}

type Check struct {
	SLO           []SLOCheck
	IntegrationID integrations.ID
}

type CheckReport struct {
	Now    time.Time
	Checks []Check
}

type IntegrationSLO struct {
	ID         integrations.ID
	HttpApiSLO *HttpApiSLO
}

type IntegrationsScope struct {
	Integrations     map[integrations.ID]*IntegrationSLO
	LastObservedTime time.Time

	sloRepository IntegrationSLORepository
	checkReporter ChecksReporter
}

func NewEmptyIntegrationsScope(sloRepository IntegrationSLORepository, checkReporter ChecksReporter) *IntegrationsScope {
	return &IntegrationsScope{
		Integrations:     make(map[integrations.ID]*IntegrationSLO),
		LastObservedTime: time.Time{},

		sloRepository: sloRepository,
		checkReporter: checkReporter,
	}
}

type CheckMessage struct {
	Now time.Time
}

func (m *CheckMessage) Execute(ctx context.Context, scope *IntegrationsScope) {
	if m.Now.After(scope.LastObservedTime) {
		scope.LastObservedTime = m.Now
	}

	var checks []Check
	for id, integration := range scope.Integrations {
		metrics := integration.HttpApiSLO.Check(scope.LastObservedTime)
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

type HttpReqsMessage struct {
	ID  integrations.ID
	Now time.Time

	Reqs []*HttpRequest
}

func (m *HttpReqsMessage) GetMessageID() []byte {
	return []byte(m.ID)
}

func (m *HttpReqsMessage) Execute(ctx context.Context, scope *IntegrationsScope) {
	if m.Now.After(scope.LastObservedTime) {
		scope.LastObservedTime = m.Now
	}

	slo, found := scope.Integrations[m.ID]
	if !found {
		slo = scope.sloRepository.GetIntegrationSLO(ctx, m.ID)
		if slo == nil {
			return
		}
		scope.Integrations[m.ID] = slo
	}
	for _, req := range m.Reqs {
		slo.HttpApiSLO.AddRequest(scope.LastObservedTime, req)
	}
}
