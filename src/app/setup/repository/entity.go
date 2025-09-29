package repository

import (
	"context"
	"hotline/integrations"
	"hotline/servicelevels"
)

type SLODefinitionRepository interface {
	servicelevels.SLODefinitionReader
	SetConfig(ctx context.Context, id integrations.ID, slo *servicelevels.HttpApiSLODefinition)
}
