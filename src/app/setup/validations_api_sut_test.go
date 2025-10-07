package setup_test

import (
	"app/setup/config"
	"context"
	"encoding/json"
	"fmt"

	"github.com/cucumber/godog"
)

type ValidationsAPISut struct {
	apiURL func() string
}

func NewValidationsAPISut(apiURL func() string) *ValidationsAPISut {
	return &ValidationsAPISut{
		apiURL: apiURL,
	}
}

func (a *ValidationsAPISut) AddSteps(sctx *godog.ScenarioContext) {
	sctx.Step(`validations for integration "([^"]*)" list is`, a.checkSchemaList)
	sctx.Step(`validation for integration "([^"]*)" is created`, a.createValidations)
	sctx.Step(`validation for integration "([^"]*)" with routeKey "([^"]*)" is deleted`, a.deleteValidation)
}

func (a *ValidationsAPISut) deleteValidation(ctx context.Context, integrationID string, routeKey string) (context.Context, error) {
	configClient, createClientErr := config.NewClientWithResponses(a.apiURL())
	if createClientErr != nil {
		return ctx, createClientErr
	}

	resp, respErr := configClient.DeleteRequestValidationWithResponse(ctx, routeKey, &config.DeleteRequestValidationParams{
		XIntegrationId: integrationID,
	})
	if respErr != nil {
		return ctx, respErr
	}

	if resp.StatusCode() != 204 {
		return ctx, fmt.Errorf("%w status code: %d", errUnexpectedResponse, resp.StatusCode())
	}

	return ctx, nil
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
	eqErr := ObjectsAreEqual(expectedSchemas, schemaList, "validations do not match, response body", string(listResponse.Body))
	if eqErr != nil {

		return ctx, eqErr
	}
	return ctx, nil
}
