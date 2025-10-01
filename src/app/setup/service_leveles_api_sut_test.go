package setup_test

import (
	"app/setup/config"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/cucumber/godog"
	"github.com/stretchr/testify/assert"
)

type ServiceLevelsAPISut struct {
	apiURL func() string
	t      *testing.T
}

func NewServiceLevelsAPISut(t *testing.T, apiURL func() string) *ServiceLevelsAPISut {
	return &ServiceLevelsAPISut{
		apiURL: apiURL,
		t:      t,
	}
}

func (a *ServiceLevelsAPISut) AddSteps(sctx *godog.ScenarioContext) {
	sctx.Step(`service levels for "([^"]*)" is set to`, a.setServiceLevelsConfiguration)
	sctx.Step(`service levels for "([^"]*)" and routeKey "([^"]*)" are deleted`, a.deleteSLOConfiguration)
	sctx.Step(`service levels for "([^"]*)" are`, a.checkServiceLevelsConfiguration)
}

func (a *ServiceLevelsAPISut) setServiceLevelsConfiguration(ctx context.Context, integrationID string, configRaw string) (context.Context, error) {
	routeRaws := strings.Split(configRaw, "|||")

	configClient, createClientErr := config.NewClient(a.apiURL())
	if createClientErr != nil {
		return ctx, createClientErr
	}

	for _, routeRaw := range routeRaws {
		input := strings.TrimSpace(routeRaw)
		if len(input) == 0 {
			continue
		}
		var reqObj config.UpsertServiceLevelsJSONRequestBody
		unmarshalErr := json.Unmarshal([]byte(input), &reqObj)
		if unmarshalErr != nil {
			return ctx, unmarshalErr
		}
		resp, responseErr := configClient.UpsertServiceLevels(
			ctx,
			&config.UpsertServiceLevelsParams{XIntegrationId: integrationID},
			reqObj)

		if responseErr != nil {
			return ctx, responseErr
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
			return ctx, fmt.Errorf("%w slo upsert: %d", errUnexpectedResponse, resp.StatusCode)
		}
	}

	return ctx, nil
}

func (a *ServiceLevelsAPISut) deleteSLOConfiguration(ctx context.Context, integrationID string, routeKey string) (context.Context, error) {
	configClient, createClientErr := config.NewClientWithResponses(a.apiURL())
	if createClientErr != nil {
		return ctx, createClientErr
	}

	resp, responseErr := configClient.DeleteServiceLevels(
		ctx,
		routeKey,
		&config.DeleteServiceLevelsParams{XIntegrationId: integrationID})

	if responseErr != nil {
		return ctx, responseErr
	}

	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return ctx, fmt.Errorf("%w status code: %d", errUnexpectedResponse, resp.StatusCode)
	}

	return ctx, nil
}

func (a *ServiceLevelsAPISut) checkServiceLevelsConfiguration(ctx context.Context, integrationID string, configRaw string) (context.Context, error) {
	routeRaws := strings.Split(configRaw, "|||")
	var routesExpected []string
	for i, routeRaw := range routeRaws {
		routeRaws[i] = strings.TrimSpace(routeRaw)
		if len(routeRaws[i]) != 0 {
			routesExpected = append(routesExpected, routeRaws[i])
		}
	}

	configClient, createClientErr := config.NewClientWithResponses(a.apiURL())
	if createClientErr != nil {
		return ctx, createClientErr
	}

	resp, responseErr := configClient.ListServiceLevelsWithResponse(
		ctx,
		&config.ListServiceLevelsParams{XIntegrationId: integrationID})

	if responseErr != nil {
		return ctx, responseErr
	}

	var routes []config.RouteServiceLevels
	if resp.StatusCode() != 200 && resp.StatusCode() != 404 {
		return ctx, fmt.Errorf("%w status code: %d", errUnexpectedResponse, resp.StatusCode())
	}
	if resp.JSON200 != nil {
		routes = resp.JSON200.Routes
	}

	if len(routes) != len(routesExpected) {
		return ctx, fmt.Errorf("%w expected %d  routes, got %d", errUnexpectedResponse, len(routesExpected), len(routes))
	}

	for i, routeExpected := range routesExpected {
		var reqObj config.RouteServiceLevels
		unmarshalErr := json.Unmarshal([]byte(routeExpected), &reqObj)
		if unmarshalErr != nil {
			return ctx, unmarshalErr
		}

		if !assert.Equal(a.t, reqObj, routes[i], "request route at index ", i, " is not equal to expected route") {
			return ctx, errConfigDoNotMatch
		}
	}

	return ctx, nil
}
