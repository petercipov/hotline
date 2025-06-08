package setup

import (
	"crypto/rand"
	"fmt"
	"hotline/clock"
	"hotline/concurrency"
	"hotline/ingestions"
	"hotline/ingestions/egress"
	"hotline/ingestions/otel"
	"hotline/reporters"
	"hotline/servicelevels"
	"hotline/uuid"
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
	EgressHttpIngestion struct {
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
	managedTime   clock.ManagedTime

	stopTick func()

	otelIngestionServer   HttpServer
	egressIngestionServer HttpServer
}

func NewApp(cfg *Config, managedTime clock.ManagedTime, createServer CreateServer, sloConfigRepository servicelevels.IntegrationSLORepository) (*App, error) {
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

	sloPipeline := servicelevels.NewSLOPipeline(sloPipelineScopes, sloConfigRepository, reporter)

	converter := otel.NewProtoConverter()
	otelHandler := otel.NewTracesHandler(func(requests []*ingestions.HttpRequest) {
		sloRequests := ingestions.ToSLORequestMessage(requests, managedTime.Now())
		sloPipeline.IngestHttpRequests(sloRequests...)
	}, converter)

	otelIngestionServer := createServer(cfg.OtelHttpIngestion.Host, otelHandler)

	egressTransport := &http.Transport{}
	uuidGenerator := uuid.NewDeterministicV7(
		managedTime.Now,
		rand.Reader)

	egressHandler := egress.New(
		egressTransport,
		func(req *ingestions.HttpRequest) {
			sloRequest := ingestions.ToSLOSingleRequestMessage(req, managedTime.Now())
			sloPipeline.IngestHttpRequests(sloRequest)
		},
		managedTime,
		60*time.Second,
		uuidGenerator,
		&egress.DefaultRequestSemantics,
	)

	egressIngestionServer := createServer(cfg.EgressHttpIngestion.Host, egressHandler)

	return &App{
		cfg:                   cfg,
		sloPipeline:           sloPipeline,
		managedTime:           managedTime,
		otelIngestion:         otelHandler,
		otelReporter:          reporter,
		otelIngestionServer:   otelIngestionServer,
		egressIngestionServer: egressIngestionServer,
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
	a.otelIngestionServer.Start()
	slog.Info("Started OTEL ingestion server", slog.String("otel-url", a.otelIngestionServer.Host()))

	a.egressIngestionServer.Start()
	slog.Info("Started Egress ingestion server", slog.String("egress-url", a.egressIngestionServer.Host()))
}

func (a *App) GetOTELIngestionUrl() string {
	return "http://" + a.otelIngestionServer.Host()
}

func (a *App) Stop() error {
	if a == nil {
		return nil
	}
	stopErr := a.otelIngestionServer.Close()
	if stopErr != nil {
		return stopErr
	}
	slog.Info("Closed ingestion server")
	if a.stopTick != nil {
		a.stopTick()
	}

	return nil
}

func (a *App) GetEgressIngestionUrl() string {
	return "http://" + a.egressIngestionServer.Host()
}

func createIds(prefix string, count int) []string {
	var queueIDs []string
	for i := 0; i < count; i++ {
		queueIDs = append(queueIDs, fmt.Sprintf("%s%d", prefix, i))
	}
	return queueIDs
}
