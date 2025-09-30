package setup_test

import (
	"app/setup"
	"app/setup/config"
	"app/setup/repository"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hotline/clock"
	"hotline/schemas"
	"hotline/servicelevels"
	"hotline/uuid"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/cucumber/godog"
)

type appSut struct {
	t *testing.T

	cfg          setup.Config
	app          *setup.App
	managedClock *clock.ManualClock

	collectorServer    setup.HttpServer
	egressTargetServer setup.HttpServer

	ingestionClient *OTELClient
	envoyClient     *EnvoyClient
	egressClient    *EgressClient

	fakeCollector    *fakeCollector
	fakeEgressTarget *fakeEgressTarget

	serviceLevelsRepository repository.ServiceLevelsRepository
	schemaRepository        repository.SchemaRepository
}

func newAppSut(t *testing.T) *appSut {
	manualClock := clock.NewManualClock(
		clock.ParseTime("2025-02-22T12:02:10Z"),
		500*time.Microsecond)
	collector := &fakeCollector{}
	target := newFakeEgressTarget(manualClock, 1234)

	uuidGenerator := uuid.NewDeterministicV7(&uuid.ConstantRandReader{})

	return &appSut{
		t:                       t,
		fakeCollector:           collector,
		fakeEgressTarget:        target,
		serviceLevelsRepository: &servicelevels.InMemoryRepository{},
		schemaRepository:        schemas.NewInMemorySchemaRepository(uuidGenerator),
		managedClock:            manualClock,
		collectorServer:         setup.NewHttpTestServer(collector),
		egressTargetServer:      setup.NewHttpTestServer(target),
	}
}

func (a *appSut) otelIngestionIsEnabled() {
	a.cfg.OtelHttpIngestion.Host = "localhost"
}

func (a *appSut) egressIngestionIsEnabled() {
	a.cfg.EgressHttpIngestion.Host = "localhost"

	a.egressTargetServer.Start()
}

func (a *appSut) serviceLevelsReporterIsPointingToCollector() {
	a.collectorServer.Start()
	a.cfg.OtelHttpReporter.Host = a.collectorServer.Host()
}

func (a *appSut) sendEgressTraffic(ctx context.Context, integrationID string) (context.Context, error) {
	now := a.managedClock.Now()
	targetURL := "http://" + a.egressTargetServer.Host()
	for range 1000 {
		a.managedClock.Reset(now)
		_, sendErr := a.egressClient.SendTraffic(integrationID, targetURL)
		if sendErr != nil {
			return ctx, sendErr
		}
	}
	return ctx, nil
}

var errUnexpectedResponse = errors.New("unexpected response")

func (a *appSut) setServiceLevelsConfiguration(ctx context.Context, integrationID string, configRaw string) (context.Context, error) {
	routeRaws := strings.Split(configRaw, "|||")

	configClient, createClientErr := config.NewClient(a.app.GetCfgAPIUrl())
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

var errConfigDoNotMatch = errors.New("configs do not match")

func (a *appSut) checkServiceLevelsConfiguration(ctx context.Context, integrationID string, configRaw string) (context.Context, error) {
	routeRaws := strings.Split(configRaw, "|||")
	var routesExpected []string
	for i, routeRaw := range routeRaws {
		routeRaws[i] = strings.TrimSpace(routeRaw)
		if len(routeRaws[i]) != 0 {
			routesExpected = append(routesExpected, routeRaws[i])
		}
	}

	configClient, createClientErr := config.NewClientWithResponses(a.app.GetCfgAPIUrl())
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

func (a *appSut) deleteSLOConfiguration(ctx context.Context, integrationID string, routeKey string) (context.Context, error) {
	configClient, createClientErr := config.NewClientWithResponses(a.app.GetCfgAPIUrl())
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

func (a *appSut) sendOTELTraffic(ctx context.Context, flavour string, integrationID string) (context.Context, error) {
	now := a.managedClock.Now()

	var statusCode int
	var sendErr error
	switch flavour {
	case "envoy otel":
		statusCode, sendErr = a.envoyClient.SendSomeTraffic(now, integrationID)
	default:
		statusCode, sendErr = a.ingestionClient.SendSomeTraffic(now, integrationID)
	}

	if sendErr != nil {
		return ctx, sendErr
	}

	if statusCode != http.StatusCreated {
		return ctx, fmt.Errorf("%w unexpected status code: %d", errUnexpectedResponse, statusCode)
	}

	nowString := now.UTC().String()
	return godog.Attach(ctx, godog.Attachment{
		FileName:  "current.time: " + nowString,
		MediaType: "text/plain",
	}), nil
}

func (a *appSut) advanceTime(ctx context.Context, seconds int) (context.Context, error) {
	a.managedClock.Advance(time.Duration(seconds) * time.Second)
	nowString := a.managedClock.Now().UTC().String()
	return godog.Attach(ctx, godog.Attachment{
		FileName:  "current.time: " + nowString,
		MediaType: "text/plain",
	}), nil
}

func (a *appSut) startHotline() error {
	app, appErr := setup.NewApp(
		&a.cfg,
		a.managedClock,
		func(_ string, handler http.Handler) setup.HttpServer {
			return setup.NewHttpTestServer(handler)
		},
		a.serviceLevelsRepository,
		a.schemaRepository,
	)
	if appErr != nil {
		return appErr
	}
	a.app = app
	a.app.Start()

	a.ingestionClient = &OTELClient{
		URL: a.app.GetOTELIngestionUrl(),
	}

	a.envoyClient = &EnvoyClient{
		URL: a.app.GetOTELIngestionUrl(),
	}

	a.app.GetEgressIngestionUrl()

	proxyURL, _ := url.Parse(a.app.GetEgressIngestionUrl())
	a.egressClient = NewEgressClient(
		proxyURL,
		1234,
	)
	return nil
}

func (a *appSut) shutdownHotline() error {
	collectorErr := a.collectorServer.Close()
	if collectorErr != nil {
		return collectorErr
	}

	egressErr := a.egressTargetServer.Close()
	if egressErr != nil {
		return egressErr
	}

	appStopErr := a.app.Stop()
	if appStopErr != nil {
		return appStopErr
	}

	return nil
}

func (a *appSut) serviceLevelsMetricsAreReceivedInCollector(ctx context.Context, metrics *godog.Table) (context.Context, error) {
	return a.fakeCollector.ExpectCollectorMetrics(ctx, a.t, metrics)
}

func (a *appSut) checkSchemaList(ctx context.Context, configRaw string) (context.Context, error) {
	configClient, createClientErr := config.NewClientWithResponses(a.app.GetCfgAPIUrl())
	if createClientErr != nil {
		return ctx, createClientErr
	}

	listResponse, listErr := configClient.ListRequestSchemasWithResponse(ctx)
	if listErr != nil {
		return ctx, listErr
	}

	if listResponse.StatusCode() != 200 && listResponse.StatusCode() != 404 {
		return ctx, fmt.Errorf("%w status code: %d", errUnexpectedResponse, listResponse.StatusCode())
	}

	schemaList := *listResponse.JSON200

	var expectedSchemas config.ListRequestSchemas
	jsonErr := json.Unmarshal([]byte(configRaw), &expectedSchemas)
	if jsonErr != nil {
		return ctx, jsonErr
	}
	if !assert.Equal(a.t, expectedSchemas, schemaList, "schemas do not match") {
		return ctx, errConfigDoNotMatch
	}
	return ctx, nil
}

func (a *appSut) createSchema(ctx context.Context, filePath string) (context.Context, error) {
	configClient, createClientErr := config.NewClientWithResponses(a.app.GetCfgAPIUrl())
	if createClientErr != nil {
		return ctx, createClientErr
	}
	schemaFile, openErr := os.Open(filepath.Clean(filePath))
	if openErr != nil {
		return ctx, openErr
	}
	defer func() {
		_ = schemaFile.Close()
	}()

	buff, _ := io.ReadAll(schemaFile)
	createSchema, createErr := configClient.CreateRequestSchemaWithBodyWithResponse(
		ctx,
		"application/octet-stream",
		bytes.NewReader(buff),
	)
	if createErr != nil {
		return ctx, createErr
	}

	if createSchema.StatusCode() != 201 {
		return ctx, fmt.Errorf("%w status code: %d", errUnexpectedResponse, createSchema.StatusCode())
	}

	if !assert.NotEmpty(a.t, createSchema.JSON201.SchemaID) {
		return ctx, errConfigDoNotMatch
	}
	if !assert.NotEmpty(a.t, createSchema.JSON201.UpdatedAt) {
		return ctx, errConfigDoNotMatch
	}

	return ctx, nil
}

func (a *appSut) compareSchemaContent(ctx context.Context, schemaID string, expectedFilePath string) (context.Context, error) {
	configClient, createClientErr := config.NewClientWithResponses(a.app.GetCfgAPIUrl())
	if createClientErr != nil {
		return ctx, createClientErr
	}

	schemaContent, getErr := configClient.GetRequestSchemaWithResponse(
		ctx,
		schemaID,
	)
	if getErr != nil {
		return ctx, getErr
	}

	if schemaContent.StatusCode() != 200 {
		return ctx, fmt.Errorf("%w status code: %d", errUnexpectedResponse, schemaContent.StatusCode())
	}

	assert.NotEmpty(a.t, schemaContent.HTTPResponse.Header.Get("Last-Modified"))
	assert.Equal(a.t, "application/octet-stream", schemaContent.HTTPResponse.Header.Get("Content-Type"))

	expectedContent, readExpectedErr := os.ReadFile(filepath.Clean(expectedFilePath))
	if readExpectedErr != nil {
		return ctx, readExpectedErr
	}

	receivedBody := string(schemaContent.Body)
	expectedBody := string(expectedContent)
	if !assert.Equal(a.t, expectedBody, receivedBody) {
		return ctx, errConfigDoNotMatch
	}

	return ctx, nil
}

func (a *appSut) schemaIsUpsertedFromFile(ctx context.Context, schemaID string, schemaFilePath string) (context.Context, error) {
	configClient, createClientErr := config.NewClientWithResponses(a.app.GetCfgAPIUrl())
	if createClientErr != nil {
		return ctx, createClientErr
	}

	schemaFile, openErr := os.Open(filepath.Clean(schemaFilePath))
	if openErr != nil {
		return ctx, openErr
	}
	defer func() {
		_ = schemaFile.Close()
	}()

	buff, _ := io.ReadAll(schemaFile)

	resp, respErr := configClient.PutRequestSchemaWithBodyWithResponse(
		ctx,
		schemaID,
		"application/octet-stream",
		bytes.NewReader(buff),
	)
	if respErr != nil {
		return ctx, respErr
	}
	if resp.StatusCode() != http.StatusCreated {
		return ctx, fmt.Errorf("%w status code: %d", errUnexpectedResponse, resp.StatusCode())
	}
	return ctx, nil
}

func (a *appSut) deleteSchema(ctx context.Context, schemaID string) (context.Context, error) {
	configClient, createClientErr := config.NewClientWithResponses(a.app.GetCfgAPIUrl())
	if createClientErr != nil {
		return ctx, createClientErr
	}

	response, deleteErr := configClient.DeleteRequestSchemaWithResponse(ctx, schemaID)
	if deleteErr != nil {
		return ctx, deleteErr
	}

	if response.StatusCode() != 204 {
		return ctx, fmt.Errorf("%w status code: %d", errUnexpectedResponse, response.StatusCode())
	}
	return ctx, nil
}

func TestApp(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: func(sctx *godog.ScenarioContext) {
			sut := newAppSut(t)
			sctx.After(func(ctx context.Context, sc *godog.Scenario, err error) (context.Context, error) {
				closeErr := sut.shutdownHotline()
				if closeErr != nil {
					return ctx, closeErr
				}
				return ctx, err
			})

			sctx.Given("OTEL ingestion is enabled", sut.otelIngestionIsEnabled)
			sctx.Given("Egress ingestion is enabled", sut.egressIngestionIsEnabled)
			sctx.Given("service levels reporter is pointing to collector", sut.serviceLevelsReporterIsPointingToCollector)
			sctx.Given("hotline is running", sut.startHotline)
			sctx.Given(`service levels configuration for "([^"]*)" is set to`, sut.setServiceLevelsConfiguration)

			sctx.When(`([^"]*) otel traffic is sent for ingestion for integration ID "([^"]*)"`, sut.sendOTELTraffic)
			sctx.When("advance time by (\\d+)s", sut.advanceTime)
			sctx.When(`egress traffic is sent for proxying for integration ID "([^"]*)"`, sut.sendEgressTraffic)
			sctx.When(`service levels configuration for "([^"]*)" and routeKey "([^"]*)" is deleted`, sut.deleteSLOConfiguration)

			sctx.Then("service levels metrics are received in collector", sut.serviceLevelsMetricsAreReceivedInCollector)
			sctx.Then(`service levels configuration for "([^"]*)" is`, sut.checkServiceLevelsConfiguration)

			sctx.Step(`schema list is`, sut.checkSchemaList)
			sctx.Step(`schema is created from file "([^"]*)"`, sut.createSchema)
			sctx.Step(`schema content for "([^"]*)" is same as in file "([^"]*)"`, sut.compareSchemaContent)
			sctx.Step(`schema "([^"]*)" is deleted`, sut.deleteSchema)
			sctx.Step(`schema "([^"]*)" is upserted from file "([^"]*)"`, sut.schemaIsUpsertedFromFile)
		},
		Options: &godog.Options{
			Format:   "pretty",
			Strict:   true,
			Paths:    []string{"features"},
			TestingT: t,
		},
	}
	suite.Run()
}
