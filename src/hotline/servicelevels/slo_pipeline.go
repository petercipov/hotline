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
	fanOut        *concurrency.FanOut[any, IntegrationsByQueue]
	sloRepository IntegrationSLORepository
	checkReporter ChecksReporter
}

func NewSLOPipeline(scopes *concurrency.Scopes[IntegrationsByQueue], sloRepository IntegrationSLORepository, checkReporter ChecksReporter) *SLOPipeline {
	p := &SLOPipeline{
		sloRepository: sloRepository,
		checkReporter: checkReporter,
	}
	fanOut := concurrency.NewFanOut(scopes, p.process)
	p.fanOut = fanOut
	return p
}

func (p *SLOPipeline) process(ctx context.Context, m any, scope *IntegrationsByQueue) {
	if checkMessage, isCheckMessage := m.(*CheckMessage); isCheckMessage {
		if checkMessage.Now.After(scope.LastObservedTime) {
			scope.LastObservedTime = checkMessage.Now
		}
		p.processCheck(ctx, scope)
	}

	if httpMessage, isHttpMessage := m.(*HttpReqsMessage); isHttpMessage {
		if httpMessage.Now.After(scope.LastObservedTime) {
			scope.LastObservedTime = httpMessage.Now
		}
		p.processHttpReqMessage(ctx, scope, httpMessage.ID, httpMessage.Reqs)
	}
}

func (p *SLOPipeline) processHttpReqMessage(ctx context.Context, scope *IntegrationsByQueue, id integrations.ID, reqs []*HttpRequest) {
	slo, found := scope.Integrations[id]
	if !found {
		slo = p.sloRepository.GetIntegrationSLO(ctx, id)
		if slo == nil {
			return
		}
		scope.Integrations[id] = slo
	}
	for _, req := range reqs {
		slo.HttpApiSLO.AddRequest(scope.LastObservedTime, req)
	}
}

func (p *SLOPipeline) processCheck(ctx context.Context, scope *IntegrationsByQueue) {
	var checks []Check
	for id, integration := range scope.Integrations {
		metrics := integration.HttpApiSLO.Check(scope.LastObservedTime)
		checks = append(checks, Check{
			SLO:           metrics,
			IntegrationID: id,
		})
	}

	p.checkReporter.ReportChecks(ctx, &CheckReport{
		Now:    scope.LastObservedTime,
		Checks: checks,
	})
}

func (p *SLOPipeline) IngestHttpRequests(m *HttpReqsMessage) {
	p.fanOut.Send([]byte(m.ID), m)
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

type IntegrationsByQueue struct {
	Integrations     map[integrations.ID]*IntegrationSLO
	LastObservedTime time.Time
}

func NewEmptyIntegrationsScope(_ context.Context) *IntegrationsByQueue {
	return &IntegrationsByQueue{
		Integrations:     make(map[integrations.ID]*IntegrationSLO),
		LastObservedTime: time.Time{},
	}
}

type CheckMessage struct {
	Now time.Time
}

type HttpReqsMessage struct {
	ID  integrations.ID
	Now time.Time

	Reqs []*HttpRequest
}
