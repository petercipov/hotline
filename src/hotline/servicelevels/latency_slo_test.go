package servicelevels_test

import (
	"fmt"
	"hotline/clock"
	"hotline/servicelevels"
	"math/rand"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
)

var _ = Describe("LatencyMs SLO", func() {
	sut := latencySLOSUT{}
	Context("no input data", func() {
		It("should return return no current metric", func() {
			sut.forEmptySLO()
			metric := sut.getMetrics()
			Expect(metric).To(HaveLen(0))
		})
	})

	Context("p50, few incremental values", func() {
		It("should return non zero metric", func() {
			sut.forEmptySLO()
			sut.WithValues(100, 200, 300, 400)
			metric := sut.getMetrics()[0]
			Expect(metric.Metric.Value).To(BeNumerically(">", 0))
		})

		It("should return p50 metric", func() {
			sut.forEmptySLO()
			sut.WithValues(100, 200, 300, 400, 500)
			metric := sut.getMetrics()[0]
			Expect(metric.Metric.Value).Should(BeInInterval(308, 309))
		})

		It("should compute metric in 1 minute p50 window SLO", func() {
			sut.forSLO([]servicelevels.PercentileDefinition{
				{Percentile: servicelevels.P50, Threshold: 5000}}, 1*time.Minute)
			sut.WithValues(100, 200, 300, 400, 500)
			metric := sut.getMetrics()[0]
			Expect(metric.Metric.Name).Should(Equal("p50"))
			Expect(metric.Metric.Value).Should(BeInInterval(308, 309))
		})
	})

	Context("For random latencies", func() {
		It("compute p99 latency metric of 1 min window, has to be <= 5s", func() {
			sut.forSLO([]servicelevels.PercentileDefinition{
				{Percentile: servicelevels.P99, Threshold: 5000}}, 1*time.Minute)
			sut.WithRandomValues(100000, 5000)
			metric := sut.getMetrics()[0]
			Expect(metric.Metric.Value).Should(BeNumerically("<=", 5000))
		})

		It("compute p50, p70, p90, p99, p999 latency metric of 1 min window, has to be <= 5s", func() {
			sut.forSLO(
				[]servicelevels.PercentileDefinition{
					{Percentile: servicelevels.P50, Threshold: 2530},
					{Percentile: servicelevels.P70, Threshold: 3530},
					{Percentile: servicelevels.P90, Threshold: 4530},
					{Percentile: servicelevels.P99, Threshold: 4960},
					{Percentile: servicelevels.P999, Threshold: 5000}},
				1*time.Minute)
			sut.WithRandomValues(100000, 5000)
			metrics := sut.getMetrics()
			Expect(metrics[0].Metric.Value).Should(BeNumerically("==", 2530))
			Expect(metrics[1].Metric.Value).Should(BeNumerically("==", 3530))
			Expect(metrics[2].Metric.Value).Should(BeNumerically("==", 4530))
			Expect(metrics[3].Metric.Value).Should(BeNumerically("==", 4960))
			Expect(metrics[4].Metric.Value).Should(BeNumerically("==", 5000))
		})
	})

	Context("For latencies over threshold 5s", func() {
		It("compute p99 with slo breach", func() {
			sut.forSLO(
				[]servicelevels.PercentileDefinition{
					{Percentile: servicelevels.P99, Threshold: 5000}},
				1*time.Minute)
			sut.WithRandomValues(1000, 10000)
			metrics := sut.getMetrics()
			Expect(metrics[0].Metric.Value).Should(BeNumerically(">=", 10000))
			Expect(metrics[0].Breach).NotTo(BeNil())
			Expect(*metrics[0].Breach).To(Equal(servicelevels.SLOBreach{
				ThresholdValue: 5000,
				ThresholdUnit:  "ms",
				Operation:      servicelevels.OperationL,
				WindowDuration: 1 * time.Minute,
			}))
		})
	})
})

type latencySLOSUT struct {
	slo *servicelevels.LatencySLO
}

func (s *latencySLOSUT) forEmptySLO() {
	s.forSLO([]servicelevels.PercentileDefinition{
		{Percentile: servicelevels.P50, Threshold: 5000}},
		1*time.Minute)
}

func (s *latencySLOSUT) getMetrics() []servicelevels.SLOCheck {
	now := clock.ParseTime("2025-02-22T12:04:55Z")
	return s.slo.Check(now)
}

func (s *latencySLOSUT) WithValues(latencies ...servicelevels.LatencyMs) {
	now := clock.ParseTime("2025-02-22T12:04:05Z")
	for _, latency := range latencies {
		s.slo.AddLatency(now, latency)
	}
}

func (s *latencySLOSUT) forSLO(percentiles []servicelevels.PercentileDefinition, duration time.Duration) {
	for i := range percentiles {
		percentiles[i].Name = fmt.Sprintf("p%g", percentiles[i].Percentile*100)
	}

	s.slo = servicelevels.NewLatencySLO(percentiles, duration, "test-namespace", nil)
}

func (s *latencySLOSUT) WithRandomValues(count int, max float64) {
	now := clock.ParseTime("2025-02-22T12:04:05Z")
	r := rand.New(rand.NewSource(10000))
	for range count {
		value := r.Float64() * max
		s.slo.AddLatency(now, servicelevels.LatencyMs(value))
	}
}

func BeInInterval(start float64, end float64) types.GomegaMatcher {
	return And(
		BeNumerically(">=", start),
		BeNumerically("<=", end),
	)
}
