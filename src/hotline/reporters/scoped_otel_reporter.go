package reporters

import (
	"compress/gzip"
	"context"
	"fmt"
	"google.golang.org/protobuf/proto"
	"hotline/concurrency"
	"hotline/servicelevels"
	"io"
	"log/slog"
	"net/http"
	"time"
)

type ScopedOtelReporter struct {
	workers *concurrency.ScopeWorkers[OtelReporterScope, OtelReporter, servicelevels.CheckReport]
}

type OtelReporterScope struct {
	client *http.Client
	gzip   *gzip.Writer
}

func NewEmptyOtelReporterScope(_ context.Context) *OtelReporterScope {
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
		func(cxt context.Context, scope *OtelReporterScope) *OtelReporter {
			scope.client = DefaultOtelHttpClient(sleep)
			scope.gzip = gzip.NewWriter(io.Discard)

			userAgent := fmt.Sprintf("%s-%s", cfg.UserAgent, concurrency.GetScopeIDFromContext(cxt))
			scopedConfig := *cfg
			scopedConfig.UserAgent = userAgent

			return NewOtelReporter(&scopedConfig, scope.client, scope.gzip, proto.Marshal)
		},
		func(ctx context.Context, _ *OtelReporterScope, worker *OtelReporter, message *servicelevels.CheckReport) {
			reportErr := worker.ReportChecks(ctx, message)
			if reportErr != nil {
				slog.ErrorContext(ctx, "Failed to report SLO checks ", slog.Any("error", reportErr))
			}
		},
		inputChannelLength)

	return &ScopedOtelReporter{
		workers: workers,
	}
}

func (r *ScopedOtelReporter) ReportChecks(_ context.Context, check *servicelevels.CheckReport) {
	r.workers.Execute(check)
}
