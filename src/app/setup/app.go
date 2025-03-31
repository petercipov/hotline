package setup

import (
	"fmt"
	"hotline/concurrency"
	"hotline/ingestions"
	"hotline/ingestions/otel"
	"hotline/reporters"
	"hotline/servicelevels"
	"log/slog"
	"net/http"
	"time"
)

type Config struct {
	OtelHttpReporter struct {
		Secured bool
		Host    string
	}
	OtelHttpIngestion struct {
		Host string
	}
	SloPipeline struct {
		CheckPeriod time.Duration
	}
}

type App struct {
	cfg *Config

	sloPipeline   *servicelevels.SLOPipeline
	otelIngestion *otel.TracesHandler
	otelReporter  *reporters.ScopedOtelReporter
	managedTime   ManagedTime

	stopTick func()

	ingestionServer HttpServer
}

func NewApp(cfg *Config, managedTime ManagedTime, createServer CreateServer) (*App, error) {
	sloPipelineScopes := concurrency.NewScopes(
		createIds("slo-queue-", 8),
		servicelevels.NewEmptyIntegrationsScope)
	otelReporterScopes := concurrency.NewScopes(
		createIds("otel-reporter-", 8),
		reporters.NewEmptyOtelReporterScope)

	oUrl, urlErr := reporters.NewOtelUrl(cfg.OtelHttpReporter.Secured, cfg.OtelHttpReporter.Host)
	if urlErr != nil {
		return nil, urlErr
	}
	reporterCfg := &reporters.OtelReporterConfig{
		OtelUrl:   oUrl,
		UserAgent: "hotline",
		Method:    http.MethodPost,
	}
	reporter := reporters.NewScopedOtelReporter(otelReporterScopes, managedTime.Sleep, reporterCfg, 100)
	fakeRepository := new(FakeSLOConfigRepository)
	sloPipeline := servicelevels.NewSLOPipeline(sloPipelineScopes, fakeRepository, reporter)

	otelTraceHttpIngestion := otel.NewTracesHandler(func(requests []*ingestions.HttpRequest) {
		sloRequests := ingestions.ToSLORequest(requests, managedTime.Now())
		sloPipeline.IngestHttpRequests(sloRequests...)
	}, otel.DefaultAttributeNames)

	ingestionServer := createServer(cfg.OtelHttpIngestion.Host, otelTraceHttpIngestion)

	return &App{
		cfg:             cfg,
		sloPipeline:     sloPipeline,
		managedTime:     managedTime,
		otelIngestion:   otelTraceHttpIngestion,
		otelReporter:    reporter,
		ingestionServer: ingestionServer,
	}, nil
}

func (a *App) Start() {
	checkPeriod := 10 * time.Second
	if a.cfg.SloPipeline.CheckPeriod != 0 {
		checkPeriod = a.cfg.SloPipeline.CheckPeriod
	}
	a.stopTick = a.managedTime.TickPeriodically(checkPeriod, func(currentTime time.Time) {
		a.sloPipeline.Check(&servicelevels.CheckMessage{
			Now: currentTime,
		})
		slog.Info("Finished check of metrics ", slog.Time("now", currentTime))
	})
	a.ingestionServer.Start()
	slog.Info("Started ingestion server", slog.String("url", a.ingestionServer.Host()))
}

func (a *App) GetIngestionUrl() string {
	return "http://" + a.ingestionServer.Host()
}

func (a *App) Stop() error {
	ingestionStopErr := a.ingestionServer.Close()
	if ingestionStopErr != nil {
		return ingestionStopErr
	}
	slog.Info("Closed ingestion server")
	if a.stopTick != nil {
		a.stopTick()
	}

	return nil
}

func createIds(prefix string, count int) []string {
	var queueIDs []string
	for i := 0; i < count; i++ {
		queueIDs = append(queueIDs, fmt.Sprintf("%s%d", prefix, i))
	}
	return queueIDs
}
