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
	ReportChecks(ctx context.Context, report CheckReport)
}

type Pipeline struct {
	publisher concurrency.PartitionPublisher
}

func NewPipeline(publisher concurrency.PartitionPublisher) *Pipeline {
	p := &Pipeline{
		publisher: publisher,
	}
	return p
}

func (p *Pipeline) IngestHttpRequest(ctx context.Context, m *IngestRequestsMessage) {
	p.publisher.PublishToPartition(ctx, m)
}

func (p *Pipeline) Check(ctx context.Context, m *CheckMessage) {
	p.publisher.PublishToPartition(ctx, m)
}

func (p *Pipeline) RouteModified(ctx context.Context, m *ModifyForRouteMessage) {
	p.publisher.PublishToPartition(ctx, m)
}

func (p *Pipeline) RequestValidated(ctx context.Context, m *RequestValidatedMessage) {
	p.publisher.PublishToPartition(ctx, m)
}

type Check struct {
	Levels        []LevelsCheck
	IntegrationID integrations.ID
}

type CheckReport []Check

type SLOScope struct {
	Integrations     map[integrations.ID]*IntegrationServiceLevels
	LastObservedTime time.Time

	sloRepository Reader
	checkReporter ChecksReporter
}

func (scope *SLOScope) AdvanceTime(now time.Time) {
	if now.After(scope.LastObservedTime) {
		scope.LastObservedTime = now
	}
}

func (scope *SLOScope) EnsureServiceLevels(ctx context.Context, id integrations.ID, createdAt time.Time) (*IntegrationServiceLevels, error) {
	slo, found := scope.Integrations[id]
	if !found {
		config, getErr := scope.sloRepository.GetServiceLevels(ctx, id)
		if getErr != nil {
			return nil, getErr
		}
		slo = NewHttpApiServiceLevels(config, createdAt)
		scope.Integrations[id] = slo
	}

	return slo, nil
}

func NewEmptyIntegrationsScope(sloRepository Reader, checkReporter ChecksReporter) *SLOScope {
	return &SLOScope{
		Integrations:     make(map[integrations.ID]*IntegrationServiceLevels),
		LastObservedTime: time.Time{},

		sloRepository: sloRepository,
		checkReporter: checkReporter,
	}
}

type CheckMessage struct {
	Now time.Time
}

func (message *CheckMessage) GetShardingKey() concurrency.ShardingKey {
	return nil
}

func (message *CheckMessage) Execute(ctx context.Context, _ string, scope *SLOScope) {
	scope.AdvanceTime(message.Now)

	var checks []Check
	for id, integration := range scope.Integrations {
		metrics := integration.Check(scope.LastObservedTime)
		checks = append(checks, Check{
			Levels:        metrics,
			IntegrationID: id,
		})
	}

	scope.checkReporter.ReportChecks(ctx, checks)
}

type IngestRequestsMessage struct {
	ID  integrations.ID
	Now time.Time

	Reqs []*HttpRequest
}

func (message *IngestRequestsMessage) GetShardingKey() concurrency.ShardingKey {
	return []byte(message.ID)
}

func (message *IngestRequestsMessage) Execute(ctx context.Context, _ string, scope *SLOScope) {
	scope.AdvanceTime(message.Now)

	slo, ensureErr := scope.EnsureServiceLevels(ctx, message.ID, message.Now)
	if ensureErr != nil {
		return
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
		slo.UpsertRoute(routeConfig, message.Now)
	} else {
		slo.DeleteRoute(message.Route)
	}
}

func (message *ModifyForRouteMessage) GetShardingKey() concurrency.ShardingKey {
	return []byte(message.ID)
}

type RequestValidatedMessage struct {
	ID  integrations.ID
	Now time.Time

	Locator http.RequestLocator
	Status  ValidationStatus
}

func (m *RequestValidatedMessage) Execute(_ context.Context, _ string, scope *SLOScope) {
	scope.AdvanceTime(m.Now)

	slo, ensureErr := scope.EnsureServiceLevels(context.Background(), m.ID, m.Now)
	if ensureErr != nil {
		return
	}

	slo.AddRequestValidation(scope.LastObservedTime, m.Locator, m.Status)
}

func (m *RequestValidatedMessage) GetShardingKey() concurrency.ShardingKey {
	return []byte(m.ID)
}

type EventsHandler struct {
	Pipeline *Pipeline
}

func (h *EventsHandler) HandleRouteModified(ctx context.Context, messages []ModifyForRouteMessage) error {
	for _, m := range messages {
		h.Pipeline.RouteModified(ctx, &m)
	}
	return nil
}
