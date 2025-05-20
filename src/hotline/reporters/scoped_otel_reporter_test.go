package reporters_test

import (
	"context"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"hotline/clock"
	"hotline/concurrency"
	"hotline/reporters"
	"hotline/servicelevels"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"time"
)

var _ = Describe("Scoped OTEL Reporter", func() {
	sut := scopedReporterSUT{}
	AfterEach(sut.Close)

	It("reports messages concurrently", func() {
		sut.forConcurrentReporter()
		for i := 0; i < 1000; i++ {
			sut.sendCommand()
		}
		reported := sut.expectReportedMetrics(1000)
		Expect(reported).NotTo(BeEmpty())
		groupByAgent := make(map[string]int)
		for _, metric := range reported {
			groupByAgent[metric.userAgent]++
		}
		Expect(len(groupByAgent)).To(Equal(8))
	})

	It("swallows reporting failed from server", func() {
		sut.forConcurrentReporter()
		sut.backendRespondsError()
		for i := 0; i < 10; i++ {
			sut.sendCommand()
		}
		reported := sut.expectReportedMetrics(10)
		Expect(reported).NotTo(BeEmpty())
	})
})

type scopedReporterSUT struct {
	mux              sync.Mutex
	testServer       *httptest.Server
	receivedMessages []reportedMetrics
	reporter         *reporters.ScopedOtelReporter
	statusCode       int
}

func (r *scopedReporterSUT) forConcurrentReporter() {
	r.statusCode = http.StatusOK
	r.testServer = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		r.mux.Lock()
		r.receivedMessages = append(r.receivedMessages, reportedMetrics{
			userAgent: request.UserAgent(),
		})
		r.mux.Unlock()

		writer.WriteHeader(r.statusCode)
	}))

	otelReporterScopes := concurrency.NewScopes(
		[]string{"1", "2", "3", "4", "5", "6", "7", "8"},
		reporters.NewEmptyOtelReporterScope)
	testServerUrl, parseErr := url.Parse(r.testServer.URL)
	Expect(parseErr).NotTo(HaveOccurred())

	otelUrl, parseOtelErr := reporters.NewOtelUrl(false, testServerUrl.Host)
	Expect(parseOtelErr).NotTo(HaveOccurred())

	r.reporter = reporters.NewScopedOtelReporter(
		otelReporterScopes, func(_ time.Duration) {
			time.Sleep(1 * time.Microsecond)
		}, &reporters.OtelReporterConfig{
			OtelUrl:   otelUrl,
			Method:    http.MethodPost,
			UserAgent: "hotline",
		}, 100)
}

func (r *scopedReporterSUT) sendCommand() {
	r.reporter.ReportChecks(context.Background(), &servicelevels.CheckReport{
		Now:    clock.ParseTime("2025-02-22T12:04:05Z"),
		Checks: simpleSLOCheck(),
	})
}

type reportedMetrics struct {
	userAgent string
}

func (r *scopedReporterSUT) getReportedMetric() []reportedMetrics {
	r.mux.Lock()
	m := r.receivedMessages
	r.mux.Unlock()
	return m
}

func (r *scopedReporterSUT) expectReportedMetrics(count int) []reportedMetrics {
	attempt := 0
	for {
		metrics := r.getReportedMetric()
		if len(metrics) >= count {
			return metrics
		}
		time.Sleep(1 * time.Millisecond)
		attempt++
		if attempt > 1000 {
			Fail("reached max attempts")
			return nil
		}
	}
}

func (r *scopedReporterSUT) Close() {
	r.testServer.Close()
	r.receivedMessages = nil
}

func (r *scopedReporterSUT) backendRespondsError() {
	r.statusCode = http.StatusInternalServerError
}
