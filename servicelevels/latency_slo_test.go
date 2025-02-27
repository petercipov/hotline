package servicelevels_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"hotline/servicelevels"
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

func (s *latencySLOSUT) forSLO(percentile float64, duration time.Duration) {
	s.slo = servicelevels.NewLatencySLO(percentile, duration)
}
