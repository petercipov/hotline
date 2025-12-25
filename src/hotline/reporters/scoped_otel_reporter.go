package reporters

import (
	"compress/gzip"
	"context"
	"fmt"
	"hotline/concurrency"
	"hotline/servicelevels"
	"io"
	"log/slog"
	"net/http"
	"time"

	"google.golang.org/protobuf/proto"
)

type ScopedOtelReporter struct {
	workers *concurrency.ScopeWorkers[OtelReporterScope, OtelReporter, servicelevels.CheckReport]
}

type OtelReporterScope struct {
	client *http.Client
	gzip   *gzip.Writer
}

func NewEmptyOtelReporterScope() *OtelReporterScope {
	return &OtelReporterScope{}
}

func NewScopedOtelReporter(
	scopes *concurrency.Scopes[OtelReporterScope],
	sleep func(t time.Duration),
	cfg *OtelReporterConfig,
	inputChannelLength int,
) *ScopedOtelReporter {
	workers := concurrency.NewScopeWorkers(
		scopes,
		func(queueID concurrency.ScopeID, scope *OtelReporterScope) *OtelReporter {
			scope.client = DefaultOtelHttpClient(sleep)
			scope.gzip = gzip.NewWriter(io.Discard)

			userAgent := fmt.Sprintf("%s-%s", cfg.UserAgent, queueID)
			scopedConfig := *cfg
			scopedConfig.UserAgent = userAgent

			return NewOtelReporter(&scopedConfig, scope.client, scope.gzip, proto.Marshal)
		},
		func(ctx context.Context, _ string, _ *OtelReporterScope, worker *OtelReporter, message *servicelevels.CheckReport) {
			reportErr := worker.ReportChecks(ctx, *message)
			if reportErr != nil {
				slog.ErrorContext(ctx, "Failed to report SLO checks ", slog.Any("error", reportErr))
			}
		},
		inputChannelLength)

	return &ScopedOtelReporter{
		workers: workers,
	}
}

func (r *ScopedOtelReporter) ReportChecks(_ context.Context, check servicelevels.CheckReport) {
	r.workers.Execute(&check)
}
