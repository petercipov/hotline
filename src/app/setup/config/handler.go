package config

import (
	"app/setup/repository"
	"context"
	"encoding/json"
	"errors"
	"hotline/clock"
	hotlinehttp "hotline/http"
	"hotline/integrations"
	"hotline/schemas"
	"hotline/servicelevels"
	"io"
	"log/slog"
	"net/http"
)

type valueIntegrationID struct{}

type HttpHandler struct {
	serviceLevelsRepo repository.ServiceLevelsRepository
	schemasRepo       repository.SchemaRepository
	nowFunc           clock.NowFunc

	routeUpserted func(integrationID integrations.ID, route hotlinehttp.Route)
}

func NewHttpHandler(
	serviceLevelsRepo repository.ServiceLevelsRepository,
	schemasRepo repository.SchemaRepository,
	nowFunc clock.NowFunc,
	routeUpserted func(integrationID integrations.ID, route hotlinehttp.Route),
) *HttpHandler {
	return &HttpHandler{
		serviceLevelsRepo: serviceLevelsRepo,
		schemasRepo:       schemasRepo,
		nowFunc:           nowFunc,
		routeUpserted:     routeUpserted,
	}
}

func (h *HttpHandler) ListSchemas(writer http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	var list = ListRequestSchemas{
		Schemas: []SchemaListEntry{},
	}

	schemaList := h.schemasRepo.ListSchemas(ctx)

	for _, schema := range schemaList {
		list.Schemas = append(list.Schemas, SchemaListEntry{
			SchemaID:  schema.ID.String(),
			UpdatedAt: schema.UpdatedAt,
		})
	}

	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusOK)
	encodeErr := json.NewEncoder(writer).Encode(list)
	if encodeErr != nil {
		slog.Error("Failed to encode response body", slog.Any("error", encodeErr))
	}
}

func (h *HttpHandler) CreateSchema(writer http.ResponseWriter, req *http.Request) {
	now := h.nowFunc()
	ctx := req.Context()
	schemaID, generateErr := h.schemasRepo.GenerateID(now)
	if generateErr != nil {
		writeResponse(ctx, writer, http.StatusInternalServerError, Error{
			Code:    "internal_error",
			Message: "Schema ID could not be created",
		})
		return
	}
	defer func() {
		_ = req.Body.Close()
	}()
	content, readErr := io.ReadAll(req.Body)
	if readErr != nil {
		writeResponse(ctx, writer, http.StatusInternalServerError, Error{
			Code:    "internal_error",
			Message: "Could not read schema content",
		})
		return
	}

	setErr := h.schemasRepo.SetSchema(ctx, schemaID, string(content), now)
	if setErr != nil {
		var validationErr *schemas.ValidationError
		isValidationErr := errors.As(setErr, &validationErr)
		if isValidationErr {
			writeResponse(ctx, writer, http.StatusBadRequest, Error{
				Code:    "bad_request",
				Message: validationErr.Error(),
			})
		} else {
			writeResponse(ctx, writer, http.StatusInternalServerError, Error{
				Code:    "internal_error",
				Message: "Could not store schema",
			})
		}
		return
	}

	response := RequestSchemaCreated{
		SchemaID:  ptrString(schemaID.String()),
		UpdatedAt: &now,
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusCreated)
	encodeErr := json.NewEncoder(writer).Encode(response)
	if encodeErr != nil {
		slog.Error("Failed to encode response body", slog.Any("error", encodeErr))
	}
}

func (h *HttpHandler) DeleteSchema(writer http.ResponseWriter, req *http.Request, schemaID SchemaID) {
	ctx := req.Context()
	setErr := h.schemasRepo.DeleteSchema(ctx, schemas.ID(schemaID))
	if setErr != nil {
		if errors.Is(setErr, io.EOF) {
			writeResponse(ctx, writer, http.StatusNotFound, Error{
				Code:    "not_found",
				Message: "schema not found",
			})
		} else {
			writeResponse(ctx, writer, http.StatusInternalServerError, Error{
				Code:    "internal_error",
				Message: "Could delete schema",
			})
		}
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (h *HttpHandler) GetSchema(writer http.ResponseWriter, req *http.Request, schemaID SchemaID) {
	ctx := req.Context()
	schemaEntry, getErr := h.schemasRepo.GetSchemaByID(ctx, schemas.ID(schemaID))
	if getErr != nil {
		if errors.Is(getErr, io.EOF) {
			writeResponse(ctx, writer, http.StatusNotFound, Error{
				Code:    "not_found",
				Message: "schema not found",
			})
		} else {
			writeResponse(ctx, writer, http.StatusInternalServerError, Error{
				Code:    "internal_error",
				Message: "Could retrieve schema",
			})
		}
		return
	}
	writer.Header().Set("Content-Type", "application/octet-stream")
	writer.Header().Set("Last-Modified", schemaEntry.UpdatedAt.UTC().Format(http.TimeFormat))
	writer.WriteHeader(http.StatusOK)
	_, writeErr := writer.Write([]byte(schemaEntry.Content))
	if writeErr != nil {
		slog.Error("Failed to write response body", slog.Any("error", writeErr))
	}
}

func (h *HttpHandler) UploadSchemaFile(_ http.ResponseWriter, _ *http.Request, _ SchemaID) {
	panic("implement me")
}

func (h *HttpHandler) GetRequestValidations(_ http.ResponseWriter, _ *http.Request, _ GetRequestValidationsParams) {
	panic("implement me")
}

func (h *HttpHandler) UpsertRequestValidations(_ http.ResponseWriter, _ *http.Request, _ UpsertRequestValidationsParams) {
	panic("implement me")
}

func (h *HttpHandler) DeleteRequestValidation(_ http.ResponseWriter, _ *http.Request, _ RouteKey, _ DeleteRequestValidationParams) {
	panic("implement me")
}

type APIEvents interface {
	RouteUpserted(integrationID integrations.ID, route hotlinehttp.Route)
}

func (h *HttpHandler) GetServiceLevels(writer http.ResponseWriter, req *http.Request, params GetServiceLevelsParams) {
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

	config := h.serviceLevelsRepo.GetConfig(ctx, integrationID)
	if config == nil {
		writeResponse(ctx, writer, http.StatusNotFound, Error{
			Code:    "not_found",
			Message: "SLO config not found",
		})
		return
	}

	resp := ServiceLevelsList{
		Routes: convertRoutes(config.Routes),
	}

	writeResponse(ctx, writer, http.StatusOK, resp)
}
func (h *HttpHandler) UpsertServiceLevels(writer http.ResponseWriter, req *http.Request, params UpsertServiceLevelsParams) {
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
	var request UpsertServiceLevelsRequest
	unmarshalErr := json.Unmarshal(buf, &request)
	if unmarshalErr != nil {
		slog.Error("Could not unmarshal request body", slog.String("integration-id", string(integrationID)), slog.Any("error", unmarshalErr))
		writeResponse(ctx, writer, http.StatusBadRequest, Error{
			Code:    "invalid_request",
			Message: "Could not unmarshal request body",
		})
		return
	}

	routeDefinition, routeErr := ParseRoute(request.Latency, request.Status, request.Route)
	if routeErr != nil {
		slog.Error("Could not parse route", slog.String("integration-id", string(integrationID)), slog.Any("error", routeErr))
		writeResponse(ctx, writer, http.StatusBadRequest, Error{
			Code:    "invalid_request",
			Message: "Could not parse route",
		})
	}

	definition := h.serviceLevelsRepo.GetConfig(ctx, integrationID)
	if definition == nil {
		definition = &servicelevels.HttpApiServiceLevels{}
	}

	definition.Upsert(routeDefinition)

	h.serviceLevelsRepo.SetConfig(ctx, integrationID, definition)
	h.routeUpserted(integrationID, routeDefinition.Route)
	key := routeDefinition.Route.ID()
	writeResponse(ctx, writer, http.StatusOK, UpsertedServiceLevelsResponse{
		RouteKey: &key,
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
func (h *HttpHandler) DeleteServiceLevels(writer http.ResponseWriter, req *http.Request, key RouteKey, params DeleteServiceLevelsParams) {
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

	config := h.serviceLevelsRepo.GetConfig(ctx, integrationID)
	if config == nil {
		writeResponse(ctx, writer, http.StatusNotFound, Error{
			Code:    "not_found",
			Message: "SLO config not found",
		})
		return
	}

	route, deleted := config.DeleteRouteByKey(key)
	h.serviceLevelsRepo.SetConfig(ctx, integrationID, config)

	if deleted {
		h.routeUpserted(integrationID, route)
	}

	writeResponse(ctx, writer, http.StatusNoContent, nil)
}
