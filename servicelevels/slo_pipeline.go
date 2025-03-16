package servicelevels

import (
	"context"
	"fmt"
	"hotline/concurrency"
	"hotline/integrations"
	"time"
)

type IntegrationSLORepository interface {
	GetIntegrationSLO(ctx context.Context, id integrations.ID) *IntegrationSLO
}

type SLOChecksReporter interface {
	ReportChecks(ctx context.Context, report CheckReport)
}

type SLOPipeline struct {
	fanOut        *concurrency.FanOut[any, *integrationsByQueue]
	sloRepository IntegrationSLORepository
	checkReporter SLOChecksReporter
}

func NewSLOPipeline(sloRepository IntegrationSLORepository, checkReporter SLOChecksReporter, numberOfQueues int) *SLOPipeline {
	var queueIDs []string
	for i := 0; i < numberOfQueues; i++ {
		queueIDs = append(queueIDs, fmt.Sprintf("queue-%d", i))
	}

	p := &SLOPipeline{
		sloRepository: sloRepository,
		checkReporter: checkReporter,
	}
	fanOut := concurrency.NewFanOut(
		queueIDs,
		p.process,
		func(_ context.Context) *integrationsByQueue {
			return &integrationsByQueue{
				integrations:     make(map[integrations.ID]*IntegrationSLO),
				lastObservedTime: time.Time{},
			}
		})
	p.fanOut = fanOut
	return p
}

func (p *SLOPipeline) process(ctx context.Context, m any, scope *integrationsByQueue) {
	if checkMessage, isCheckMessage := m.(CheckMessage); isCheckMessage {
		if checkMessage.Now.After(scope.lastObservedTime) {
			scope.lastObservedTime = checkMessage.Now
		}
		p.processCheck(ctx, scope)
	}

	if httpMessage, isHttpMessage := m.(HttpReqsMessage); isHttpMessage {
		if httpMessage.Now.After(scope.lastObservedTime) {
			scope.lastObservedTime = httpMessage.Now
		}
		p.processHttpReqMessage(ctx, scope, httpMessage.ID, httpMessage.Reqs)
	}
}

func (p *SLOPipeline) processHttpReqMessage(ctx context.Context, scope *integrationsByQueue, id integrations.ID, reqs []*HttpRequest) {
	slo, found := scope.integrations[id]
	if !found {
		slo = p.sloRepository.GetIntegrationSLO(ctx, id)
		if slo == nil {
			return
		}
		scope.integrations[id] = slo
	}
	for _, req := range reqs {
		slo.HttpApiSLO.AddRequest(scope.lastObservedTime, req)
	}
}

func (p *SLOPipeline) processCheck(ctx context.Context, scope *integrationsByQueue) {
	var checks []Check
	for id, integration := range scope.integrations {
		metrics := integration.HttpApiSLO.Check(scope.lastObservedTime)
		checks = append(checks, Check{
			SLO:           metrics,
			IntegrationID: id,
		})
	}

	p.checkReporter.ReportChecks(ctx, CheckReport{
		Now:    scope.lastObservedTime,
		Checks: checks,
	})
}

func (p *SLOPipeline) IngestHttpRequests(m HttpReqsMessage) {
	p.fanOut.Send([]byte(m.ID), m)
}

func (p *SLOPipeline) Check(m CheckMessage) {
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

type integrationsByQueue struct {
	integrations     map[integrations.ID]*IntegrationSLO
	lastObservedTime time.Time
}

type CheckMessage struct {
	Now time.Time
}

type HttpReqsMessage struct {
	ID  integrations.ID
	Now time.Time

	Reqs []*HttpRequest
}
