package setup_test

import (
	"app/setup/config"
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/cucumber/godog"
	"github.com/stretchr/testify/assert"
)

type ValidationsAPISut struct {
	apiURL func() string
	t      *testing.T
}

func NewValidationsAPISut(t *testing.T, apiURL func() string) *ValidationsAPISut {
	return &ValidationsAPISut{
		apiURL: apiURL,
		t:      t,
	}
}

func (a *ValidationsAPISut) AddSteps(sctx *godog.ScenarioContext) {
	sctx.Step(`validations for integration "([^"]*)" list is`, a.checkSchemaList)
	sctx.Step(`validation for integration "([^"]*)" is created`, a.createValidations)
}

func (a *ValidationsAPISut) createValidations(ctx context.Context, integrationID string, configRaw string) (context.Context, error) {
	configClient, createClientErr := config.NewClientWithResponses(a.apiURL())
	if createClientErr != nil {
		return ctx, createClientErr
	}

	var body config.UpsertRequestValidationsJSONRequestBody
	jsonErr := json.Unmarshal([]byte(configRaw), &body)
	if jsonErr != nil {
		return ctx, jsonErr
	}

	_, upsertErr := configClient.UpsertRequestValidationsWithResponse(
		ctx,
		&config.UpsertRequestValidationsParams{
			XIntegrationId: integrationID,
		},
		body)

	if upsertErr != nil {
		return ctx, upsertErr
	}

	return ctx, nil
}

func (a *ValidationsAPISut) checkSchemaList(ctx context.Context, integrationID string, configRaw string) (context.Context, error) {
	configClient, createClientErr := config.NewClientWithResponses(a.apiURL())
	if createClientErr != nil {
		return ctx, createClientErr
	}

	listResponse, listErr := configClient.ListRequestValidationsWithResponse(ctx, &config.ListRequestValidationsParams{
		XIntegrationId: integrationID,
	})
	if listErr != nil {
		return ctx, listErr
	}

	if listResponse.StatusCode() != 200 && listResponse.StatusCode() != 404 {
		return ctx, fmt.Errorf("%w status code: %d", errUnexpectedResponse, listResponse.StatusCode())
	}

	schemaList := *listResponse.JSON200

	var expectedSchemas config.RequestValidationList
	jsonErr := json.Unmarshal([]byte(configRaw), &expectedSchemas)
	if jsonErr != nil {
		return ctx, jsonErr
	}
	if !assert.Equal(a.t, expectedSchemas, schemaList, "schemas do not match") {
		return ctx, errConfigDoNotMatch
	}
	return ctx, nil
}
