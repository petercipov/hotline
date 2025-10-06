package setup_test

import (
	"app/setup"
	"app/setup/repository"
	"context"
	"errors"
	"hotline/clock"
	"hotline/schemas"
	"hotline/servicelevels"
	"hotline/uuid"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/cucumber/godog"
)

type appSut struct {
	t *testing.T

	cfg          setup.Config
	app          *setup.App
	managedClock *clock.ManualClock

	serviceLevelsRepository repository.ServiceLevelsRepository
	schemaRepository        repository.SchemaRepository
	validationRepository    repository.ValidationRepository
}

func newAppSut(t *testing.T) *appSut {
	manualClock := clock.NewManualClock(
		clock.ParseTime("2025-02-22T12:02:10Z"),
		500*time.Microsecond)

	uuidGenerator := uuid.NewV7(&uuid.ConstantRandReader{})
	return &appSut{
		t:                       t,
		serviceLevelsRepository: &servicelevels.InMemoryRepository{},
		schemaRepository:        schemas.NewInMemorySchemaRepository(uuidGenerator),
		validationRepository:    schemas.NewInMemoryValidationRepository(),
		managedClock:            manualClock,
	}
}

func (a *appSut) GetCfgAPIUrl() string {
	return a.app.GetCfgAPIUrl()
}

var errUnexpectedResponse = errors.New("unexpected response")
var errConfigDoNotMatch = errors.New("configs do not match")

func (a *appSut) advanceTime(ctx context.Context, seconds int) (context.Context, error) {
	a.managedClock.Advance(time.Duration(seconds) * time.Second)
	nowString := a.managedClock.Now().UTC().String()
	return godog.Attach(ctx, godog.Attachment{
		FileName:  "current.time: " + nowString,
		MediaType: "text/plain",
	}), nil
}

func (a *appSut) startHotline(_ context.Context, features *godog.Table) error {
	if features != nil {
		for _, row := range features.Rows[1:] {
			feature := strings.TrimSpace(row.Cells[0].Value)
			enabled := row.Cells[1].Value == "true"

			switch feature {
			case "otel ingestion":
				if enabled {
					a.cfg.OtelHttpIngestion.Host = "localhost"
				}
			case "egress ingestion":
				if enabled {
					a.cfg.EgressHttpIngestion.Host = "localhost"
				}
			}
		}
	}

	app, appErr := setup.NewApp(
		&a.cfg,
		a.managedClock,
		func(_ string, handler http.Handler) setup.HttpServer {
			return setup.NewHttpTestServer(handler)
		},
		a.serviceLevelsRepository,
		a.schemaRepository,
		a.validationRepository,
	)
	if appErr != nil {
		return appErr
	}
	a.app = app
	a.app.Start()
	return nil
}

func (a *appSut) Close() error {
	appStopErr := a.app.Stop()
	if appStopErr != nil {
		return appStopErr
	}
	return nil
}

func TestApp(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: func(sctx *godog.ScenarioContext) {
			sut := newAppSut(t)
			schemaAPISut := NewSchemaAPISut(t, sut.GetCfgAPIUrl)
			serviceLevelsAPISut := NewServiceLevelsAPISut(t, sut.GetCfgAPIUrl)
			reporterSut := NewReporterSut(t, func(url string) {
				sut.cfg.OtelHttpReporter.Host = url
			})
			egressSut := NewEgressSut(sut.managedClock, func() string {
				return sut.app.GetEgressIngestionUrl()
			})
			otelSut := NewOTELSut(sut.managedClock, func() string {
				return sut.app.GetOTELIngestionUrl()
			})
			validationsAPISut := NewValidationsAPISut(t, sut.GetCfgAPIUrl)

			sctx.After(func(ctx context.Context, sc *godog.Scenario, err error) (context.Context, error) {
				closeAppErr := sut.Close()
				if closeAppErr != nil {
					return ctx, closeAppErr
				}

				reporterErr := reporterSut.Close()
				if reporterErr != nil {
					return ctx, reporterErr
				}

				egressSutErr := egressSut.Close()
				if egressSutErr != nil {
					return ctx, egressSutErr
				}
				return ctx, err
			})

			sctx.Given("hotline is running", sut.startHotline)
			sctx.Step("advance time by (\\d+)s", sut.advanceTime)

			schemaAPISut.AddSteps(sctx)
			serviceLevelsAPISut.AddSteps(sctx)
			reporterSut.AddSteps(sctx)
			egressSut.AddSteps(sctx)
			otelSut.AddSteps(sctx)
			validationsAPISut.AddSteps(sctx)
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
