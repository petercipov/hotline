package servicelevels

import (
	"context"
	"errors"
	"hotline/clock"
	"hotline/http"
	"hotline/integrations"
)

type Repository interface {
	Reader
	Modify(ctx context.Context, id integrations.ID, slo ApiServiceLevels) error
	Drop(ctx context.Context, id integrations.ID) error
}

type RouteUpsertedPublisher interface {
	HandleRouteModified(ctx context.Context, event []ModifyForRouteMessage) error
}

type UseCase struct {
	repo      Repository
	nowFunc   clock.NowFunc
	publisher RouteUpsertedPublisher
}

func NewUseCase(repo Repository, nowFunc clock.NowFunc, publisher RouteUpsertedPublisher) *UseCase {
	return &UseCase{
		repo:      repo,
		nowFunc:   nowFunc,
		publisher: publisher,
	}
}

func (u *UseCase) GetServiceLevels(ctx context.Context, id integrations.ID) (ApiServiceLevels, error) {
	return u.repo.GetServiceLevels(ctx, id)
}

type RouteModification struct {
	Route      http.Route
	Latency    *LatencyServiceLevels
	Status     *StatusServiceLevels
	Validation *ValidationServiceLevels
}

func (u *UseCase) ModifyRoute(ctx context.Context, id integrations.ID, routeDef RouteModification) (http.RouteKey, error) {
	now := u.nowFunc()
	route := routeDef.Route.Normalize()
	key := route.GenerateKey(id.String())

	levels, getErr := u.repo.GetServiceLevels(ctx, id)
	if getErr != nil {
		if !errors.Is(getErr, ErrServiceLevelsNotFound) {
			return key, getErr
		}
	}
	levels.Upsert(RouteServiceLevels{
		Route:      route,
		Key:        key,
		Latency:    routeDef.Latency,
		Status:     routeDef.Status,
		Validation: routeDef.Validation,
	})
	setErr := u.repo.Modify(ctx, id, levels)
	if setErr != nil {
		return key, setErr
	}
	publishErr := u.publisher.HandleRouteModified(ctx, []ModifyForRouteMessage{
		{
			ID:    id,
			Route: routeDef.Route,
			Now:   now,
		},
	})
	return key, publishErr
}

func (u *UseCase) DeleteRoute(ctx context.Context, id integrations.ID, routeKey http.RouteKey) error {
	now := u.nowFunc()
	levels, getErr := u.repo.GetServiceLevels(ctx, id)
	if getErr != nil {
		return getErr
	}
	route, deleted := levels.DeleteRouteByKey(routeKey)
	if !deleted {
		return ErrRouteNotFound
	}
	setErr := u.repo.Modify(ctx, id, levels)
	if setErr != nil {
		return setErr
	}
	return u.publisher.HandleRouteModified(ctx, []ModifyForRouteMessage{
		{
			ID:    id,
			Route: route,
			Now:   now,
		},
	})
}

func (u *UseCase) DropServiceLevels(ctx context.Context, id integrations.ID) error {
	now := u.nowFunc()
	levels, getErr := u.repo.GetServiceLevels(ctx, id)
	if getErr != nil {
		if !errors.Is(getErr, ErrServiceLevelsNotFound) {
			return getErr
		}
	}

	setErr := u.repo.Drop(ctx, id)
	if setErr != nil {
		return setErr
	}
	var messages []ModifyForRouteMessage
	for _, route := range levels.Routes {
		messages = append(messages, ModifyForRouteMessage{
			ID:    id,
			Now:   now,
			Route: route.Route,
		})
	}
	return u.publisher.HandleRouteModified(ctx, messages)
}
