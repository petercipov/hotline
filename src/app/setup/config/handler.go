package config

import (
	"context"
	"encoding/json"
	hotlinehttp "hotline/http"
	"hotline/integrations"
	"hotline/servicelevels"
	"io"
	"log/slog"
	"net/http"
)

type valueIntegrationID struct{}

type HttpHandler struct {
	repository    *InMemorySLODefinitions
	routeUpserted func(integrationID integrations.ID, route hotlinehttp.Route)
}

type APIEvents interface {
	RouteUpserted(integrationID integrations.ID, route hotlinehttp.Route)
}

func NewHttpHandler(repository *InMemorySLODefinitions, routeUpserted func(integrationID integrations.ID, route hotlinehttp.Route)) *HttpHandler {
	return &HttpHandler{
		repository:    repository,
		routeUpserted: routeUpserted,
	}
}

func (h *HttpHandler) GetSLOConfig(writer http.ResponseWriter, req *http.Request, params GetSLOConfigParams) {
	ctx := req.Context()
	if len(params.XIntegrationId) == 0 {
		slog.Error("Could not find X-Integration-Id header")
		writeResponse(ctx, writer, http.StatusBadRequest, Error{
			Code:    "invalid_request",
			Message: "X-Integration-Id header is required",
		})
		return
	}
	integrationID := integrations.ID(params.XIntegrationId)
	ctx = context.WithValue(req.Context(), valueIntegrationID{}, integrationID)

	config := h.repository.GetConfig(ctx, integrationID)
	if config == nil {
		writeResponse(ctx, writer, http.StatusNotFound, Error{
			Code:    "not_found",
			Message: "SLO config not found",
		})
		return
	}

	resp := ListDefinitions{
		Routes: convertRoutes(config.Routes),
	}

	writeResponse(ctx, writer, http.StatusOK, resp)
}
func (h *HttpHandler) UpsertSLOConfig(writer http.ResponseWriter, req *http.Request, params UpsertSLOConfigParams) {
	ctx := req.Context()
	if len(params.XIntegrationId) == 0 {
		slog.Error("Could not find X-Integration-Id header")
		writeResponse(ctx, writer, http.StatusBadRequest, Error{
			Code:    "invalid_request",
			Message: "X-Integration-Id header is required",
		})
		return
	}

	integrationID := integrations.ID(params.XIntegrationId)
	ctx = context.WithValue(req.Context(), valueIntegrationID{}, integrationID)

	defer func() {
		_ = req.Body.Close()
	}()
	buf, _ := io.ReadAll(req.Body)
	var request UpsertSLORequest
	unmarshalErr := json.Unmarshal(buf, &request)
	if unmarshalErr != nil {
		slog.Error("Could not unmarshal request body", slog.String("integration-id", string(integrationID)), slog.Any("error", unmarshalErr))
		writeResponse(ctx, writer, http.StatusBadRequest, Error{
			Code:    "invalid_request",
			Message: "Could not unmarshal request body",
		})
		return
	}

	routeDefinition, routeErr := ParseRoute(request.Definition, request.Route)
	if routeErr != nil {
		slog.Error("Could not parse route", slog.String("integration-id", string(integrationID)), slog.Any("error", routeErr))
		writeResponse(ctx, writer, http.StatusBadRequest, Error{
			Code:    "invalid_request",
			Message: "Could not parse route",
		})
	}

	definition := h.repository.GetConfig(ctx, integrationID)
	if definition == nil {
		definition = &servicelevels.HttpApiSLODefinition{}
	}

	definition.Upsert(routeDefinition)

	h.repository.SetConfig(integrationID, *definition)
	h.routeUpserted(integrationID, routeDefinition.Route)
	writeResponse(ctx, writer, http.StatusOK, UpsertSLOResponse{
		RouteKey: routeDefinition.Route.ID(),
	})
}

func writeResponse(ctx context.Context, writer http.ResponseWriter, status int, value any) {
	integrationID := ctx.Value(valueIntegrationID{}).(integrations.ID)
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	raw, errMarshalErr := json.Marshal(value)
	if errMarshalErr != nil {
		slog.Error("Could not marshal response", slog.String("integration-id", string(integrationID)), slog.Any("error", errMarshalErr))
	}
	_, writeErr := writer.Write(raw)
	if writeErr != nil {
		slog.Error("Could not write error", slog.String("integration-id", string(integrationID)), slog.Any("error", writeErr))
	}
}
func (h *HttpHandler) DeleteSLOConfig(writer http.ResponseWriter, req *http.Request, key RouteKey, params DeleteSLOConfigParams) {
	ctx := req.Context()
	if len(params.XIntegrationId) == 0 {
		slog.Error("Could not find X-Integration-Id header")
		writeResponse(ctx, writer, http.StatusBadRequest, Error{
			Code:    "invalid_request",
			Message: "X-Integration-Id header is required",
		})
		return
	}
	integrationID := integrations.ID(params.XIntegrationId)
	ctx = context.WithValue(req.Context(), valueIntegrationID{}, integrationID)

	config := h.repository.GetConfig(ctx, integrationID)
	if config == nil {
		writeResponse(ctx, writer, http.StatusNotFound, Error{
			Code:    "not_found",
			Message: "SLO config not found",
		})
		return
	}

	route, deleted := config.DeleteRouteByKey(key)
	h.repository.SetConfig(integrationID, *config)

	if deleted {
		h.routeUpserted(integrationID, route)
	}

	writeResponse(ctx, writer, http.StatusNoContent, nil)
}
