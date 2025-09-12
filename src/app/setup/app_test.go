package setup_test

import (
	"app/setup"
	"app/setup/config"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hotline/clock"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/cucumber/godog"
)

type appSut struct {
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

	fakeSLOConfigRepository *config.InMemorySLODefinitions
}

func newAppSut() *appSut {
	manualClock := clock.NewManualClock(
		clock.ParseTime("2025-02-22T12:02:10Z"),
		500*time.Microsecond)
	collector := &fakeCollector{}
	target := newFakeEgressTarget(manualClock, 1234)
	fakeSLOConfigRepository := config.NewInMemorySLODefinitions()
	return &appSut{
		fakeCollector:           collector,
		fakeEgressTarget:        target,
		fakeSLOConfigRepository: fakeSLOConfigRepository,
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

func (a *appSut) sloReporterIsPointingToCollector() {
	a.collectorServer.Start()
	a.cfg.OtelHttpReporter.Host = a.collectorServer.Host()
}

func (a *appSut) sendEgressTraffic(ctx context.Context, integrationID string) (context.Context, error) {
	now := a.managedClock.Now()
	targetURL := "http://" + a.egressTargetServer.Host()
	for i := 0; i < 1000; i++ {
		a.managedClock.Reset(now)
		_, sendErr := a.egressClient.SendTraffic(integrationID, targetURL)
		if sendErr != nil {
			return ctx, sendErr
		}
	}
	return ctx, nil
}

func (a *appSut) setSLOConfiguration(ctx context.Context, integrationID string, configRaw string) (context.Context, error) {
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
			return ctx, fmt.Errorf("unexpected response for slo upsert: %d", resp.StatusCode)
		}
	}

	return ctx, nil
}

func (a *appSut) checkSLOConfiguration(ctx context.Context, integrationID string, configRaw string) (context.Context, error) {
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

	resp, responseErr := configClient.GetServiceLevelsWithResponse(
		ctx,
		&config.GetServiceLevelsParams{XIntegrationId: integrationID})

	if responseErr != nil {
		return ctx, responseErr
	}

	var routes []config.RouteServiceLevels
	if resp.StatusCode() != 200 && resp.StatusCode() != 404 {
		return ctx, errors.New(fmt.Sprint("unexpected status code: ", resp.StatusCode()))
	}
	if resp.JSON200 != nil {
		routes = resp.JSON200.Routes
	}

	if len(routes) != len(routesExpected) {
		return ctx, errors.New(fmt.Sprint("expected ", len(routesExpected), " routes, got ", len(routes)))
	}

	for i, routeExpected := range routesExpected {
		var reqObj config.RouteServiceLevels
		unmarshalErr := json.Unmarshal([]byte(routeExpected), &reqObj)
		if unmarshalErr != nil {
			return ctx, unmarshalErr
		}

		e := &errorT{}
		assert.Equal(e, reqObj, routes[i], "request route at index ", i, " is not equal to expected route")
		if e.err != nil {
			return ctx, e.err
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

	if resp.StatusCode != 204 {
		return ctx, errors.New(fmt.Sprint("unexpected status code: ", resp.StatusCode))
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
		return ctx, errors.New(fmt.Sprint("unexpected status code: ", statusCode))
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
		a.fakeSLOConfigRepository,
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

func (a *appSut) sloMetricsAreReceivedInCollector(ctx context.Context, metrics *godog.Table) (context.Context, error) {
	return a.fakeCollector.ExpectCollectorMetrics(ctx, metrics)
}

func TestApp(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: func(sctx *godog.ScenarioContext) {
			sut := newAppSut()
			sctx.After(func(ctx context.Context, sc *godog.Scenario, err error) (context.Context, error) {
				closeErr := sut.shutdownHotline()
				if closeErr != nil {
					return ctx, closeErr
				}
				return ctx, err
			})

			sctx.Given("OTEL ingestion is enabled", sut.otelIngestionIsEnabled)
			sctx.Given("Egress ingestion is enabled", sut.egressIngestionIsEnabled)
			sctx.Given("slo reporter is pointing to collector", sut.sloReporterIsPointingToCollector)
			sctx.Given("hotline is running", sut.startHotline)
			sctx.Given(`slo configuration for "([^"]*)" is set to`, sut.setSLOConfiguration)

			sctx.When(`([^"]*) otel traffic is sent for ingestion for integration ID "([^"]*)"`, sut.sendOTELTraffic)
			sctx.When("advance time by (\\d+)s", sut.advanceTime)
			sctx.When(`egress traffic is sent for proxying for integration ID "([^"]*)"`, sut.sendEgressTraffic)
			sctx.When(`slo configuration for "([^"]*)" and routeKey "([^"]*)" is deleted`, sut.deleteSLOConfiguration)

			sctx.Then("slo metrics are received in collector", sut.sloMetricsAreReceivedInCollector)
			sctx.Then(`slo configuration for "([^"]*)" is`, sut.checkSLOConfiguration)
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
