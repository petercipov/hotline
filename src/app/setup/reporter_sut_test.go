package setup_test

import (
	"app/setup"
	"context"
	"testing"

	"github.com/cucumber/godog"
)

type ReporterSut struct {
	collectorServer setup.HttpServer
	fakeCollector   *fakeCollector
	callback        urlCallback
	t               *testing.T
}

type urlCallback func(url string)

func NewReporterSut(t *testing.T, callback urlCallback) *ReporterSut {
	collector := &fakeCollector{}

	return &ReporterSut{
		fakeCollector:   collector,
		callback:        callback,
		collectorServer: setup.NewHttpTestServer(collector),
		t:               t,
	}
}

func (a *ReporterSut) AddSteps(sctx *godog.ScenarioContext) {
	sctx.Given("service levels reporter is pointing to collector", a.serviceLevelsReporterIsPointingToCollector)
	sctx.Then("service levels metrics are received in collector", a.serviceLevelsMetricsAreReceivedInCollector)
}

func (a *ReporterSut) serviceLevelsReporterIsPointingToCollector() {
	a.collectorServer.Start()
	a.callback(a.collectorServer.Host())
}

func (a *ReporterSut) serviceLevelsMetricsAreReceivedInCollector(ctx context.Context, metrics *godog.Table) (context.Context, error) {
	return a.fakeCollector.ExpectCollectorMetrics(ctx, a.t, metrics)
}

func (a *ReporterSut) Close() error {
	return a.collectorServer.Close()
}
