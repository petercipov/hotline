package metrics_test

import (
	"hotline/metrics"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
)

var _ = Describe("LatencyMs Histogram", func() {

	s := sutlatencyhistogram{}

	Context("P50", func() {
		It("computes 0 for an empty histogram", func() {
			s.forEmptyHistogram()
			bucket := s.computeP50()
			Expect(bucket.From).Should(BeNumerically("==", 0))
			Expect(bucket.To).Should(BeNumerically("==", 0))
		})

		It("computes 0 for histogram without enough data", func() {
			s.forEmptyHistogram()
			s.fillLatencies(17, 11)
			bucket := s.computeP50()
			Expect(bucket.From).Should(BeNumerically("==", 0))
			Expect(bucket.To).Should(BeNumerically("==", 0))
		})

		It("computes bucket for a 3 latencies", func() {
			s.forEmptyHistogram()
			s.fillLatencies(17, 11, 22)
			bucket := s.computeP50()
			Expect(bucket.From).Should(BeInInterval(16.36, 16.37))
			Expect(bucket.To).Should(BeInInterval(18.82, 18.83))
		})

		It("computes median as middle values for long series", func() {
			s.forEmptyHistogram()
			s.repeatIncreasingLatencies(10000, 1000)
			Expect(s.h.SizeInBytes()).Should(BeNumerically("<=", 1000))
			bucket := s.computeP50()
			Expect(bucket.From).Should(BeInInterval(4383.9, 4384))
			Expect(bucket.To).Should(BeInInterval(5041, 5042))
		})

		It("computes median as middle values for long series and size in bytes is low", func() {
			s.forEmptyHistogram()
			s.repeatIncreasingLatencies(10000000, 1)
			Expect(s.h.SizeInBytes()).Should(BeNumerically("<=", 1000))
		})

		It("computes median more precise if split buckets by exact threshold", func() {
			s.forEmptyHistogramWithSplit(1000)
			s.fillLatencies(500, 1000, 1000, 1000, 1000, 1900)
			bucket := s.computeP50()
			Expect(bucket.From).Should(BeInInterval(942, 943))
			Expect(bucket.To).Should(BeNumerically("==", 1000))
		})

		It("computes median more precise if split buckets by exact threshold. near thresholds share same bucket", func() {
			s.forEmptyHistogramWithSplit(1000, 1050)
			s.fillLatencies(500, 1000, 1000, 1000, 1000, 1900)
			bucket := s.computeP50()
			Expect(bucket.From).Should(BeInInterval(942, 943))
			Expect(bucket.To).Should(BeNumerically("==", 1000))
		})

		It("computes median less precise if split buckets without exact threshold", func() {
			s.forEmptyHistogram()
			s.fillLatencies(500, 1000, 1000, 1000, 1000, 1900)
			bucket := s.computeP50()
			Expect(bucket.From).Should(BeInInterval(942, 943))
			Expect(bucket.To).Should(BeInInterval(1083, 1084))
		})

		It("computes median more precise if split buckets by exact thresholds", func() {
			s.forEmptyHistogramWithSplit(2000, 1000)
			s.fillLatencies(500, 1000, 1000, 1000, 1000, 1900)
			bucket := s.computeP50()
			Expect(bucket.From).Should(BeInInterval(942, 943))
			Expect(bucket.To).Should(BeNumerically("==", 1000))

			s.repeatLatencies(10, 2000, 2000, 2000, 2000)
			bucket = s.computeP50()
			Expect(bucket.From).Should(BeInInterval(1895, 1896))
			Expect(bucket.To).Should(BeNumerically("==", 2000))

		})

		It("moves small latencies into zero bucket", func() {
			s.forEmptyHistogramWithSplit()
			s.fillLatencies(0.0001, 0.00001, 0.000001, 0.0000001)
			bucket := s.computeP50()
			Expect(bucket.From).Should(BeNumerically("==", 0))
			Expect(bucket.To).Should(BeNumerically("==", 1))
		})
	})

	Context("P99", func() {
		It("computes 0 for an empty histogram", func() {
			s.forEmptyHistogram()
			bucket := s.computeP99()
			Expect(bucket.From).Should(BeNumerically("==", 0))
			Expect(bucket.To).Should(BeNumerically("==", 0))
		})

		It("computes bucket for a 3 latencies", func() {
			s.forEmptyHistogram()
			s.fillLatencies(17, 11, 22)
			bucket := s.computeP99()
			Expect(bucket.From).Should(BeInInterval(21.6, 21.7))
			Expect(bucket.To).Should(BeInInterval(24.8, 24.9))
		})
	})
})

type sutlatencyhistogram struct {
	h *metrics.LatencyHistogram
}

func (s *sutlatencyhistogram) forEmptyHistogram() {
	s.h = metrics.NewLatencyHistogram(nil)
}

func (s *sutlatencyhistogram) computeP50() metrics.Bucket {
	b, _ := s.h.ComputePercentile(0.5)
	return b
}

func (s *sutlatencyhistogram) fillLatencies(latencies ...float64) {
	for _, latency := range latencies {
		s.h.Add(latency)
	}
}

func (s *sutlatencyhistogram) repeatLatencies(repeat int, latencies ...float64) {
	for range repeat {
		s.fillLatencies(latencies...)
	}
}

func (s *sutlatencyhistogram) repeatIncreasingLatencies(count int, repeat int) []float64 {
	var latencies []float64
	for latency := 1; latency <= count; latency++ {
		latencies = append(latencies, float64(latency))
	}

	for range repeat {
		s.fillLatencies(latencies...)
	}
	return latencies
}

func (s *sutlatencyhistogram) forEmptyHistogramWithSplit(splitLatency ...float64) {
	s.h = metrics.NewLatencyHistogram(splitLatency)
}

func (s *sutlatencyhistogram) computeP99() metrics.Bucket {
	p, _ := s.h.ComputePercentile(0.99)
	return p
}

func BeInInterval(start float64, end float64) types.GomegaMatcher {
	return And(
		BeNumerically(">=", start),
		BeNumerically("<=", end),
	)
}
