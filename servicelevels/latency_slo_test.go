package servicelevels_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"hotline/servicelevels"
	"math/rand"
	"time"
)

var _ = Describe("Latency SLO", func() {
	sut := latencySLOSUT{}
	Context("no input data", func() {
		It("should return return no current metric", func() {
			sut.forEmptySLO()
			metric := sut.getMetric()
			Expect(metric).To(BeNumerically("==", 0))
		})
	})

	Context("p50, few incremental values", func() {
		It("should return non zero metric", func() {
			sut.forEmptySLO()
			sut.WithValues(100, 200, 300, 400)
			metric := sut.getMetric()
			Expect(metric).To(BeNumerically(">", 0))
		})

		It("should return p50 metric", func() {
			sut.forEmptySLO()
			sut.WithValues(100, 200, 300, 400, 500)
			metric := sut.getMetric()
			Expect(metric).Should(BeInInterval(308, 309))
		})

		It("should compute metric in 1 minute p50 window SLO", func() {
			sut.forSLO(0.5, 1*time.Minute)
			sut.WithValues(100, 200, 300, 400, 500)
			metric := sut.getMetric()
			Expect(metric).Should(BeInInterval(308, 309))
		})
	})

	Context("For latencies distributed exponentially", func() {
		It("compute p99 latency metric of 1 min window", func() {
			sut.forSLO(0.99, 1*time.Minute, 5)
			sut.WithRandomValues(1000, 5)
			metric := sut.getMetric()
			Expect(metric).Should(BeNumerically("<=", 5))
		})
	})

})

type latencySLOSUT struct {
	slo *servicelevels.LatencySLO
}

func (s *latencySLOSUT) forEmptySLO() {
	s.forSLO(0.5, 1*time.Minute)
}

func (s *latencySLOSUT) getMetric() interface{} {
	now := parseTime("2025-02-22T12:04:55Z")
	return s.slo.GetMetric(now)
}

func (s *latencySLOSUT) WithValues(latencies ...float64) {
	now := parseTime("2025-02-22T12:04:05Z")
	for _, latency := range latencies {
		s.slo.AddLatency(now, latency)
	}
}

func (s *latencySLOSUT) forSLO(percentile float64, duration time.Duration, splits ...float64) {
	s.slo = servicelevels.NewLatencySLO(percentile, duration, splits)
}

func (s *latencySLOSUT) WithRandomValues(count int, max float64) {
	now := parseTime("2025-02-22T12:04:05Z")
	for range count {
		value := rand.Float64() * max
		s.slo.AddLatency(now, value)
	}
}
