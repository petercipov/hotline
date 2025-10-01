package setup_test

import (
	"context"
	"fmt"
	"hotline/clock"
	"net/http"

	"github.com/cucumber/godog"
)

type OTELSut struct {
	clock   clock.ManagedTime
	otelUrl func() string
}

func NewOTELSut(clock clock.ManagedTime, otelUrl func() string) *OTELSut {
	return &OTELSut{
		clock:   clock,
		otelUrl: otelUrl,
	}
}

func (a *OTELSut) AddSteps(sctx *godog.ScenarioContext) {
	sctx.When(`OTEL traffic for "([^"]*)" is sent for integration ID "([^"]*)"`, a.sendOTELTraffic)
}

func (a *OTELSut) sendOTELTraffic(ctx context.Context, flavour string, integrationID string) (context.Context, error) {
	now := a.clock.Now()

	var statusCode int
	var sendErr error
	switch flavour {
	case "envoy":
		client := &EnvoyClient{
			URL: a.otelUrl(),
		}
		statusCode, sendErr = client.SendSomeTraffic(now, integrationID)
	default:
		client := &OTELClient{
			URL: a.otelUrl(),
		}
		statusCode, sendErr = client.SendSomeTraffic(now, integrationID)
	}

	if sendErr != nil {
		return ctx, sendErr
	}

	if statusCode != http.StatusCreated {
		return ctx, fmt.Errorf("%w unexpected status code: %d", errUnexpectedResponse, statusCode)
	}
	return ctx, nil
}
