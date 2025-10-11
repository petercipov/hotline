package config

import (
	"context"
	"encoding/json"
	"errors"
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
	serviceLevelsUseCases *servicelevels.UseCase
	schemasUseCases       *schemas.SchemaUseCase
	validationUseCases    *schemas.ValidationUseCase
}

func NewHttpHandler(
	serviceLevelsUseCases *servicelevels.UseCase,
	schemasUseCases *schemas.SchemaUseCase,
	validationUseCases *schemas.ValidationUseCase,
) *HttpHandler {
	return &HttpHandler{
		serviceLevelsUseCases: serviceLevelsUseCases,
		schemasUseCases:       schemasUseCases,
		validationUseCases:    validationUseCases,
	}
}

func (h *HttpHandler) ListRequestValidations(writer http.ResponseWriter, req *http.Request, params ListRequestValidationsParams) {
	var list = RequestValidationList{
		RouteValidations: []RouteRequestValidation{},
	}
	ctx, validIntegrationId, integrationID := requireIntegrationId(req.Context(), params.XIntegrationId, writer)
	if !validIntegrationId {
		return
	}

	validations, getErr := h.validationUseCases.GetValidations(ctx, integrationID)
	if getErr != nil {
		writeResponse(ctx, writer, http.StatusInternalServerError, Error{
			Code:    "internal_error",
			Message: "Could not list validations",
		})
	}
	for _, r := range validations {
		routeValidation := RouteRequestValidation{
			RequestSchema:  nil,
			ResponseSchema: nil,
			Route:          convertRoute(r.Route),
			RouteKey:       r.RouteKey.String(),
		}
		if r.Validators.Request != nil {
			routeValidation.RequestSchema = &RequestValidationSchema{
				BodySchemaID:   optSchemaID(r.Validators.Request.BodySchemaID),
				HeaderSchemaID: optSchemaID(r.Validators.Request.HeaderSchemaID),
				QuerySchemaID:  optSchemaID(r.Validators.Request.QuerySchemaID),
			}
		}
		list.RouteValidations = append(list.RouteValidations, routeValidation)
	}
	writeResponse(ctx, writer, http.StatusOK, list)
}
func (h *HttpHandler) UpsertRequestValidations(writer http.ResponseWriter, req *http.Request, params UpsertRequestValidationsParams) {
	ctx, validIntegrationId, integrationID := requireIntegrationId(req.Context(), params.XIntegrationId, writer)
	if !validIntegrationId {
		return
	}
	defer func() {
		_ = req.Body.Close()
	}()

	var v UpsertRequestValidationRequest
	decodeErr := json.NewDecoder(req.Body).Decode(&v)
	if decodeErr != nil {
		writeResponse(ctx, writer, http.StatusBadRequest, Error{
			Code:    "invalid_request",
			Message: "Could not decode request body",
		})
		return
	}

	route := parseRoute(v.Route)
	routeValidators := schemas.RouteValidators{}

	if v.RequestSchema != nil {
		routeValidators.Request = &schemas.RequestValidators{
			HeaderSchemaID: parseSchemaID(v.RequestSchema.HeaderSchemaID),
			QuerySchemaID:  parseSchemaID(v.RequestSchema.QuerySchemaID),
			BodySchemaID:   parseSchemaID(v.RequestSchema.BodySchemaID),
		}
	}

	routeKey, setErr := h.validationUseCases.UpsertValidation(ctx, integrationID, route, routeValidators)
	if setErr != nil {
		writeResponse(ctx, writer, http.StatusInternalServerError, Error{
			Code:    "internal_error",
			Message: "Could not store validation",
		})
		return
	}

	writeResponse(ctx, writer, http.StatusCreated, UpsertedRequestValidationResponse{
		RouteKey: routeKey.String(),
	})
}
func (h *HttpHandler) DeleteRequestValidation(writer http.ResponseWriter, req *http.Request, routekey RouteKey, params DeleteRequestValidationParams) {
	ctx, validIntegrationId, integrationID := requireIntegrationId(req.Context(), params.XIntegrationId, writer)
	if !validIntegrationId {
		return
	}
	defer func() {
		_ = req.Body.Close()
	}()

	deleteErr := h.validationUseCases.DeleteValidation(ctx, integrationID, hotlinehttp.RouteKey(routekey))
	if deleteErr != nil {
		writeResponse(ctx, writer, http.StatusInternalServerError, Error{
			Code:    "internal_error",
			Message: "Could not delete validation",
		})
		return
	}
	writeResponse(ctx, writer, http.StatusNoContent, nil)
}

func (h *HttpHandler) ListRequestSchemas(writer http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	defer func() {
		_ = req.Body.Close()
	}()

	var list = ListRequestSchemas{
		Schemas: []SchemaListEntry{},
	}

	schemaList, listErr := h.schemasUseCases.ListSchemas(ctx)
	if listErr != nil {
		writeResponse(ctx, writer, http.StatusInternalServerError, Error{
			Code:    "internal_error",
			Message: "Could not list schemas",
		})
		return
	}

	for _, schema := range schemaList {
		list.Schemas = append(list.Schemas, SchemaListEntry{
			SchemaID:  schema.ID.String(),
			Title:     schema.Title,
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
func (h *HttpHandler) CreateRequestSchema(writer http.ResponseWriter, req *http.Request, params CreateRequestSchemaParams) {
	ctx := req.Context()
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

	entry, setErr := h.schemasUseCases.CreateSchema(ctx, string(content), optString(params.Title, ""))
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
		SchemaID:  entry.ID.String(),
		UpdatedAt: entry.UpdatedAt,
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusCreated)
	encodeErr := json.NewEncoder(writer).Encode(response)
	if encodeErr != nil {
		slog.Error("Failed to encode response body", slog.Any("error", encodeErr))
	}
}
func (h *HttpHandler) DeleteRequestSchema(writer http.ResponseWriter, req *http.Request, schemaID SchemaID) {
	ctx := req.Context()
	setErr := h.schemasUseCases.DeleteSchema(ctx, schemas.ID(schemaID))
	if setErr != nil {
		if errors.Is(setErr, schemas.ErrSchemaNotFound) {
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
func (h *HttpHandler) GetRequestSchema(writer http.ResponseWriter, req *http.Request, schemaID SchemaID) {
	ctx := req.Context()
	schemaEntry, getErr := h.schemasUseCases.GetSchema(ctx, schemas.ID(schemaID))
	if getErr != nil {
		if errors.Is(getErr, schemas.ErrSchemaNotFound) {
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
func (h *HttpHandler) PutRequestSchema(writer http.ResponseWriter, req *http.Request, schemaID SchemaID, params PutRequestSchemaParams) {
	ctx := req.Context()
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

	modifyErr := h.schemasUseCases.ModifySchema(ctx, schemas.ID(schemaID), string(content), optString(params.Title, ""))
	if modifyErr != nil {
		var validationErr *schemas.ValidationError
		isValidationErr := errors.As(modifyErr, &validationErr)
		if isValidationErr {
			writeResponse(ctx, writer, http.StatusBadRequest, Error{
				Code:    "bad_request",
				Message: validationErr.Error(),
			})
		} else {
			if errors.Is(modifyErr, schemas.ErrSchemaNotFound) {
				writeResponse(ctx, writer, http.StatusNotFound, Error{
					Code:    "not_found",
					Message: "Request Schema not found",
				})
			} else {
				writeResponse(ctx, writer, http.StatusInternalServerError, Error{
					Code:    "internal_error",
					Message: "Could not store schema",
				})
			}
		}
		return
	}

	writer.WriteHeader(http.StatusCreated)
}

func (h *HttpHandler) ListServiceLevels(writer http.ResponseWriter, req *http.Request, params ListServiceLevelsParams) {
	ctx, validIntegrationId, integrationID := requireIntegrationId(req.Context(), params.XIntegrationId, writer)
	if !validIntegrationId {
		return
	}
	defer func() {
		_ = req.Body.Close()
	}()

	config, getErr := h.serviceLevelsUseCases.GetServiceLevels(ctx, integrationID)
	if getErr != nil {
		if !errors.Is(getErr, servicelevels.ErrServiceLevelsNotFound) {
			writeResponse(ctx, writer, http.StatusInternalServerError, Error{
				Code:    "internal_error",
				Message: "Could not list service levels",
			})
		}
	}
	resp := ServiceLevelsList{
		Routes: convertRoutes(config.Routes),
	}

	writeResponse(ctx, writer, http.StatusOK, resp)
}
func (h *HttpHandler) UpsertServiceLevels(writer http.ResponseWriter, req *http.Request, params UpsertServiceLevelsParams) {
	ctx, validIntegrationId, integrationID := requireIntegrationId(req.Context(), params.XIntegrationId, writer)
	if !validIntegrationId {
		return
	}
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

	routeDefinition, routeErr := ParseRoute(integrationID, request.Latency, request.Status, request.Route)
	if routeErr != nil {
		slog.Error("Could not parse route", slog.String("integration-id", string(integrationID)), slog.Any("error", routeErr))
		writeResponse(ctx, writer, http.StatusBadRequest, Error{
			Code:    "invalid_request",
			Message: "Could not parse route",
		})
	}

	routeKey, modifyErr := h.serviceLevelsUseCases.ModifyRoute(ctx, integrationID, routeDefinition)
	if modifyErr != nil {
		writeResponse(ctx, writer, http.StatusInternalServerError, Error{
			Code:    "internal_error",
			Message: "Could not modify service levels",
		})
		return
	}
	writeResponse(ctx, writer, http.StatusOK, UpsertedServiceLevelsResponse{
		RouteKey: routeKey.String(),
	})
}
func (h *HttpHandler) DeleteServiceLevels(writer http.ResponseWriter, req *http.Request, key RouteKey, params DeleteServiceLevelsParams) {
	ctx, validIntegrationId, integrationID := requireIntegrationId(req.Context(), params.XIntegrationId, writer)
	if !validIntegrationId {
		return
	}
	defer func() {
		_ = req.Body.Close()
	}()

	deleteErr := h.serviceLevelsUseCases.DeleteRoute(ctx, integrationID, hotlinehttp.RouteKey(key))
	if deleteErr != nil {
		if errors.Is(deleteErr, servicelevels.ErrServiceLevelsNotFound) {
			writeResponse(ctx, writer, http.StatusNotFound, Error{
				Code:    "not_found",
				Message: "Service Levels not found",
			})
			return
		}
		if errors.Is(deleteErr, servicelevels.ErrRouteNotFound) {
			writeResponse(ctx, writer, http.StatusNotFound, Error{
				Code:    "not_found",
				Message: "Route not found",
			})
			return
		}
		writeResponse(ctx, writer, http.StatusInternalServerError, Error{
			Code:    "internal_error",
			Message: "Could not delete service levels",
		})
		return
	}
	writeResponse(ctx, writer, http.StatusNoContent, nil)
}

func requireIntegrationId(ctx context.Context, raw string, writer http.ResponseWriter) (context.Context, bool, integrations.ID) {
	if len(raw) == 0 {
		slog.Error("Could not find X-Integration-Id header")
		writeResponse(ctx, writer, http.StatusBadRequest, Error{
			Code:    "invalid_request",
			Message: "X-Integration-Id header is required",
		})
		return ctx, false, ""
	}

	integrationID := integrations.ID(raw)
	ctx = context.WithValue(ctx, valueIntegrationID{}, integrationID)

	return ctx, true, integrationID
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

func optSchemaID(id *schemas.ID) *SchemaID {
	if id == nil {
		return nil
	}

	rawID := id.String()
	return &rawID
}

func parseSchemaID(id *SchemaID) *schemas.ID {
	if id == nil {
		return nil
	}

	schemaID := schemas.ID(*id)

	return &schemaID
}
