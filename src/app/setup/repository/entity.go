package repository

import (
	"context"
	"hotline/http"
	"hotline/integrations"
	"hotline/schemas"
	"hotline/servicelevels"
	"time"
)

type ServiceLevelsRepository interface {
	servicelevels.ConfigReader
	SetConfig(ctx context.Context, id integrations.ID, slo *servicelevels.HttpApiServiceLevels)
}

type SchemaRepository interface {
	schemas.SchemaReader
	GetSchemaByID(ctx context.Context, id schemas.ID) (schemas.SchemaEntry, error)
	GenerateID(now time.Time) (schemas.ID, error)
	SetSchema(ctx context.Context, id schemas.ID, content string, updateAt time.Time, title string) error
	ListSchemas(ctx context.Context) []schemas.SchemaListEntry
	DeleteSchema(ctx context.Context, id schemas.ID) error
}

type ValidationRepository interface {
	schemas.ValidationReader
	SetForRoute(ctx context.Context, id integrations.ID, route http.Route, schemaDef schemas.RouteSchemaDefinition) (http.RouteKey, error)
	DeleteRouteByKey(ctx context.Context, id integrations.ID, key http.RouteKey) error
}
