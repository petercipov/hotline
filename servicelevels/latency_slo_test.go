package servicelevels_test

import (
	"fmt"
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
			Expect(metric.Metric.Value).To(BeNumerically("==", 0))
		})
	})

	Context("p50, few incremental values", func() {
		It("should return non zero metric", func() {
			sut.forEmptySLO()
			sut.WithValues(100, 200, 300, 400)
			metric := sut.getMetric()
			Expect(metric.Metric.Value).To(BeNumerically(">", 0))
		})

		It("should return p50 metric", func() {
			sut.forEmptySLO()
			sut.WithValues(100, 200, 300, 400, 500)
			metric := sut.getMetric()
			Expect(metric.Metric.Value).Should(BeInInterval(308, 309))
		})

		It("should compute metric in 1 minute p50 window SLO", func() {
			sut.forSLO([]float64{0.5}, 1*time.Minute)
			sut.WithValues(100, 200, 300, 400, 500)
			metric := sut.getMetric()
			Expect(metric.Metric.Name).Should(Equal("p50"))
			Expect(metric.Metric.Value).Should(BeInInterval(308, 309))
		})
	})

	Context("For random latencies", func() {
		It("compute p99 latency metric of 1 min window, has to be <= 5s", func() {
			sut.forSLO([]float64{0.99}, 1*time.Minute, 5000)
			sut.WithRandomValues(100000, 5000)
			metric := sut.getMetric()
			Expect(metric.Metric.Value).Should(BeNumerically("<=", 5000))
		})

		It("compute p50, p70, p90, p99, p999 latency metric of 1 min window, has to be <= 5s", func() {
			sut.forSLO(
				[]float64{0.5, 0.7, 0.9, 0.99, 0.999},
				1*time.Minute,
				2530, 3530, 4530, 4960, 5000)
			sut.WithRandomValues(100000, 5000)
			metrics := sut.getMetrics()
			Expect(metrics[0].Metric.Value).Should(BeNumerically("==", 2530))
			Expect(metrics[1].Metric.Value).Should(BeNumerically("==", 3530))
			Expect(metrics[2].Metric.Value).Should(BeNumerically("==", 4530))
			Expect(metrics[3].Metric.Value).Should(BeNumerically("==", 4960))
			Expect(metrics[4].Metric.Value).Should(BeNumerically("==", 5000))
		})
	})
})

type latencySLOSUT struct {
	slo *servicelevels.LatencySLO
}

func (s *latencySLOSUT) forEmptySLO() {
	s.forSLO([]float64{0.5}, 1*time.Minute)
}

func (s *latencySLOSUT) getMetric() servicelevels.SLOCheck {
	return s.getMetrics()[0]
}

func (s *latencySLOSUT) getMetrics() []servicelevels.SLOCheck {
	now := parseTime("2025-02-22T12:04:55Z")
	return s.slo.Check(now)
}

func (s *latencySLOSUT) WithValues(latencies ...float64) {
	now := parseTime("2025-02-22T12:04:05Z")
	for _, latency := range latencies {
		s.slo.AddLatency(now, latency)
	}
}

func (s *latencySLOSUT) forSLO(percentiles []float64, duration time.Duration, splits ...float64) {
	var definition []servicelevels.PercentileDefinition
	for _, percentile := range percentiles {
		definition = append(definition, servicelevels.PercentileDefinition{
			Percentile: percentile,
			Name:       fmt.Sprintf("p%g", percentile*100),
			Threshold:  99.99,
		})
	}
	s.slo = servicelevels.NewLatencySLO(definition, duration, splits)
}

func (s *latencySLOSUT) WithRandomValues(count int, max float64) {
	now := parseTime("2025-02-22T12:04:05Z")
	r := rand.New(rand.NewSource(10000))
	for range count {
		value := r.Float64() * max
		s.slo.AddLatency(now, value)
	}
}
