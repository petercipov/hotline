package setup_test

import (
	"app/setup"
	"app/setup/config"
	"context"
	"errors"
	"fmt"
	"hotline/clock"
	"hotline/integrations"
	"net/http"
	"net/url"
	"testing"
	"time"

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

	fakeSLOConfigRepository *config.FakeSLOConfigRepository
}

func newAppSut() *appSut {
	manualClock := clock.NewManualClock(
		clock.ParseTime("2025-02-22T12:02:10Z"),
		500*time.Microsecond)
	collector := &fakeCollector{}
	target := newFakeEgressTarget(manualClock, 1234)
	fakeSLOConfigRepository := config.NewFakeSLOConfigRepository()
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
	definition, parseErr := config.ParseServiceLevelFromBytes([]byte(configRaw))
	if parseErr != nil {
		return ctx, parseErr
	}

	a.fakeSLOConfigRepository.SetConfig(integrations.ID(integrationID), definition)
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
			sctx.Given(`slo configuration for "([^"]*)" is`, sut.setSLOConfiguration)

			sctx.When(`([^"]*) otel traffic is sent for ingestion for integration ID "([^"]*)"`, sut.sendOTELTraffic)
			sctx.When("advance time by (\\d+)s", sut.advanceTime)
			sctx.When(`egress traffic is sent for proxying for integration ID "([^"]*)"`, sut.sendEgressTraffic)

			sctx.Then("slo metrics are received in collector", sut.sloMetricsAreReceivedInCollector)
		},
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"features"},
			TestingT: t,
		},
	}
	suite.Run()
}
