package setup

import (
	"app/setup/config"
	"app/setup/repository"
	"crypto/rand"
	"hotline/clock"
	"hotline/concurrency"
	hotlinehttp "hotline/http"
	"hotline/ingestions"
	"hotline/ingestions/egress"
	"hotline/ingestions/otel"
	"hotline/integrations"
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
	ConfigAPI struct {
		Host string
	}
}

type App struct {
	cfg *Config

	sloPipeline   *servicelevels.Pipeline
	otelIngestion *otel.TracesHandler
	otelReporter  *reporters.ScopedOtelReporter
	managedTime   clock.ManagedTime

	stopTick func()

	otelIngestionServer   HttpServer
	egressIngestionServer HttpServer
	cfgAPIServer          HttpServer
}

func NewApp(
	cfg *Config,
	managedTime clock.ManagedTime,
	createServer CreateServer,
	serviceLevelsRepository repository.ServiceLevelsRepository,
	schemaRepository repository.SchemaRepository,
) (*App, error) {
	otelReporterScopes := concurrency.NewScopes(
		concurrency.GenerateScopeIds("otel-reporter", 8),
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

	sloPipelineScopes := concurrency.NewScopes(
		concurrency.GenerateScopeIds("slo-scope", 8),
		func() *servicelevels.SLOScope {
			return servicelevels.NewEmptyIntegrationsScope(serviceLevelsRepository, reporter)
		},
	)
	sloPipeline := servicelevels.NewPipeline(sloPipelineScopes)

	converter := otel.NewProtoConverter()
	otelHandler := otel.NewTracesHandler(func(requests []*ingestions.HttpRequest) {
		sloRequests := ingestions.ToSLORequestMessage(requests, managedTime.Now())
		for _, req := range sloRequests {
			sloPipeline.IngestHttpRequest(req)
		}
	}, converter)

	otelIngestionServer := createServer(cfg.OtelHttpIngestion.Host, otelHandler)

	egressTransport := &http.Transport{}
	uuidGenerator := uuid.NewDeterministicV7(rand.Reader)

	defaultSemantics := egress.DefaultRequestSemantics()
	egressHandler := egress.New(
		egressTransport,
		func(req *ingestions.HttpRequest) {
			sloRequest := ingestions.ToSLOSingleRequestMessage(req, managedTime.Now())
			sloPipeline.IngestHttpRequest(sloRequest)
		},
		managedTime,
		60*time.Second,
		uuidGenerator,
		&defaultSemantics,
	)

	egressIngestionServer := createServer(cfg.EgressHttpIngestion.Host, egressHandler)
	cfgAPIHandler := config.HandlerWithOptions(config.NewHttpHandler(
		serviceLevelsRepository,
		schemaRepository,
		managedTime.Now,
		func(integrationID integrations.ID, route hotlinehttp.Route) {
			sloPipeline.ModifyRoute(&servicelevels.ModifyRouteMessage{
				ID:    integrationID,
				Now:   managedTime.Now(),
				Route: route,
			})
		},
	), config.StdHTTPServerOptions{})
	cfgAPIServer := createServer(cfg.ConfigAPI.Host, cfgAPIHandler)

	return &App{
		cfg:                   cfg,
		sloPipeline:           sloPipeline,
		managedTime:           managedTime,
		otelIngestion:         otelHandler,
		otelReporter:          reporter,
		otelIngestionServer:   otelIngestionServer,
		egressIngestionServer: egressIngestionServer,
		cfgAPIServer:          cfgAPIServer,
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

	a.cfgAPIServer.Start()
	slog.Info("Started Config API server", slog.String("config-api-url", a.cfgAPIServer.Host()))
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

func (a *App) GetCfgAPIUrl() string {
	return "http://" + a.cfgAPIServer.Host()
}
