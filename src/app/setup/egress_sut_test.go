package setup_test

import (
	"app/setup"
	"context"
	"hotline/clock"
	"net/url"

	"github.com/cucumber/godog"
)

type EgressSut struct {
	clock              *clock.ManualClock
	fakeEgressTarget   *fakeEgressTarget
	egressTargetServer setup.HttpServer
	callback           urlCallback

	egressUrl func() string
}

func NewEgressSut(clock *clock.ManualClock, egressUrl func() string) *EgressSut {
	target := newFakeEgressTarget(clock, 1234)

	return &EgressSut{
		clock:              clock,
		fakeEgressTarget:   target,
		egressTargetServer: setup.NewHttpTestServer(target),
		egressUrl:          egressUrl,
	}
}

func (a *EgressSut) AddSteps(sctx *godog.ScenarioContext) {
	sctx.When(`Egress traffic is sent for proxying for integration ID "([^"]*)"`, a.sendEgressTraffic)
}

func (a *EgressSut) sendEgressTraffic(ctx context.Context, integrationID string) (context.Context, error) {
	now := a.clock.Now()
	proxyURL, _ := url.Parse(a.egressUrl())
	egressClient := NewEgressClient(
		proxyURL,
		1234,
	)

	a.egressTargetServer.Start()
	targetURL := "http://" + a.egressTargetServer.Host()
	for range 1000 {
		a.clock.Reset(now)
		_, sendErr := egressClient.SendTraffic(integrationID, targetURL)
		if sendErr != nil {
			return ctx, sendErr
		}
	}
	return ctx, nil
}

func (a *EgressSut) Close() error {
	egressErr := a.egressTargetServer.Close()
	if egressErr != nil {
		return egressErr
	}
	return nil
}
