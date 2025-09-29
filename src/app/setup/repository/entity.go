package repository

import (
	"context"
	"hotline/integrations"
	"hotline/servicelevels"
)

type ServiceLevelsRepository interface {
	servicelevels.ConfigReader
	SetConfig(ctx context.Context, id integrations.ID, slo *servicelevels.HttpApiServiceLevels)
}
