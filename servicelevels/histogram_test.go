package servicelevels_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"hotline/servicelevels"
)

var _ = Describe("Histogram", func() {

	s := sut{}

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
			Expect(bucket.From).Should(BeNumerically("<=", 17))
			Expect(bucket.To).Should(BeNumerically(">=", 17))
		})

		It("computes median as middle values for long series", func() {
			s.forEmptyHistogram()
			s.repeatIncreasingLatencies(10000, 1000)
			Expect(s.h.SizeInBytes()).Should(BeNumerically("<=", 1000))
			bucket := s.computeP50()
			Expect(bucket.From).Should(BeNumerically("<=", 4384))
			Expect(bucket.To).Should(BeNumerically(">=", 4384))
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
			Expect(bucket.From).Should(BeNumerically(">=", 942))
			Expect(bucket.To).Should(BeNumerically("==", 1000))
		})

		It("computes median less precise if split buckets without exact threshold", func() {
			s.forEmptyHistogram()
			s.fillLatencies(500, 1000, 1000, 1000, 1000, 1900)
			bucket := s.computeP50()
			Expect(bucket.From).Should(BeNumerically(">=", 942))
			Expect(bucket.To).ShouldNot(BeNumerically("==", 1000))
			Expect(bucket.To).Should(BeNumerically("<=", 1084))
		})

		It("computes median more precise if split buckets by exact thresholds", func() {
			s.forEmptyHistogramWithSplit(2000, 1000)
			s.fillLatencies(500, 1000, 1000, 1000, 1000, 1900)
			bucket := s.computeP50()
			Expect(bucket.From).Should(BeNumerically(">=", 942))
			Expect(bucket.To).Should(BeNumerically("==", 1000))

			s.repeatLatencies(10, 2000, 2000, 2000, 2000)
			bucket = s.computeP50()
			Expect(bucket.From).Should(BeNumerically(">=", 1895))
			Expect(bucket.To).Should(BeNumerically("==", 2000))

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
			Expect(bucket.From).Should(BeNumerically("<=", 21.7))
			Expect(bucket.To).Should(BeNumerically(">=", 24.8))
		})
	})
})

type sut struct {
	h *servicelevels.Histogram
}

func (s *sut) forEmptyHistogram() {
	s.h = servicelevels.NewHistogram(nil)
}

func (s *sut) computeP50() servicelevels.Bucket {
	return s.h.ComputePercentile(0.5)
}

func (s *sut) fillLatencies(latencies ...float64) {
	for _, latency := range latencies {
		s.h.Add(latency)
	}
}

func (s *sut) repeatLatencies(repeat int, latencies ...float64) {
	for i := 0; i < repeat; i++ {
		s.fillLatencies(latencies...)
	}
}

func (s *sut) repeatIncreasingLatencies(count int, repeat int) []float64 {
	var latencies []float64
	for latency := 1; latency <= count; latency++ {
		latencies = append(latencies, float64(latency))
	}

	for i := 0; i < repeat; i++ {
		s.fillLatencies(latencies...)
	}
	return latencies
}

func (s *sut) forEmptyHistogramWithSplit(splitLatency ...float64) {
	s.h = servicelevels.NewHistogram(splitLatency)
}

func (s *sut) computeP99() servicelevels.Bucket {
	return s.h.ComputePercentile(0.99)
}
