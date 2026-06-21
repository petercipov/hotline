package tdigest_test

import (
	"fmt"
	"hotline/metrics/tdigest"
	"math"
	"math/rand/v2"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("TDigest", func() {
	Context("digest", func() {
		sut := tdigestSut{}

		It("will create new centroid for entry if empty digest", func() {
			sut.forTDigest()
			sut.AddEntry(3.14)

			centroids := sut.ToCentroids()
			Expect(centroids).To(HaveLen(1))
		})

		It("will update centroid same weight", func() {
			sut.forTDigest()
			sut.AddEntry(3.14)
			sut.AddEntry(10.14)
			sut.AddEntry(3.14)

			centroids := sut.ToCentroids()
			Expect(centroids).To(Equal([]tdigest.Centroid{
				{
					Mean: 3.14, Weight: 2,
				},
				{
					Mean: 10.14, Weight: 1,
				},
			}))
		})

		It("will update last centroid if same weight exactly", func() {
			sut.forTDigest()
			sut.AddEntry(3.14)
			sut.AddEntry(3.14)

			centroids := sut.ToCentroids()
			Expect(centroids).To(Equal([]tdigest.Centroid{
				{
					Mean: 3.14, Weight: 2,
				},
			}))
		})

		It("will NOT update last centroid if NOT same weight exactly", func() {
			sut.forTDigest()
			sut.AddEntry(3.14)
			sut.AddEntry(3.15)
			sut.AddEntry(3.16)

			centroids := sut.ToCentroids()
			Expect(centroids).To(Equal([]tdigest.Centroid{
				{Mean: 3.14, Weight: 1},
				{Mean: 3.15, Weight: 1},
				{Mean: 3.16, Weight: 1},
			}))
		})

		It("generate bounded centroids for 100k random numbers", func() {
			sut.forTDigest()
			sut.AddRandomEntries(100_000)

			centroids := sut.ToCentroids()
			Expect(centroids).To(HaveLen(57))

			Expect(sut.Quantile(0.70)).To(Equal(7.356594988103647))
			Expect(sut.Quantile(0.80)).To(Equal(8.368116168239936))
			Expect(sut.Quantile(0.90)).To(Equal(9.416024900604626))
			Expect(sut.Quantile(0.99)).To(Equal(10.362849014104123))

			totalWeight := uint64(0)
			for _, centroid := range centroids {
				totalWeight += centroid.Weight
			}

			Expect(totalWeight).To(Equal(uint64(100_000)))
		})

		It("apply increasing sequence of numbers with bounded array", func() {
			sut.forTDigestWithHihBuffer()
			sut.AddIncreasingEntries(100_000)

			centroids := sut.ToCentroids()
			Expect(centroids).To(HaveLen(66))

			totalWeight := uint64(0)
			for _, centroid := range centroids {
				totalWeight += centroid.Weight
			}

			Expect(totalWeight).To(Equal(uint64(100_000)))
		})

		It("apply decreasing sequence of numbers with bounded array", func() {
			sut.forTDigestWithHihBuffer()
			sut.AddDescreasingEntries(100_000)

			centroids := sut.ToCentroids()
			Expect(centroids).To(HaveLen(69))

			totalWeight := uint64(0)
			for _, centroid := range centroids {
				totalWeight += centroid.Weight
			}

			Expect(totalWeight).To(Equal(uint64(100_000)))
		})

		Context("Quantiles", func() {
			It("should compute 0 for an empty tdigest", func() {
				sut.forTDigest()

				quantile := sut.Quantile(0.99)
				Expect(quantile).To(Equal(0.0))
			})

			It("should compute same value for a single centroid", func() {
				sut.forTDigest()
				sut.AddEntry(3.14)

				quantile := sut.Quantile(0.99)
				Expect(quantile).To(Equal(3.14))
			})

			It("should use linear approximation to compute value between two centroids", func() {
				sut.forTDigest()
				sut.AddSimpleDataSet()

				quantile := sut.Quantile(0.90)
				Expect(quantile).Should(BeNumerically("~", 2.68, 0.01))
			})

			It("should use same centroid for quantiles near the start", func() {
				sut.forTDigest()
				sut.AddSimpleDataSet()

				quantile := sut.Quantile(0.00)
				Expect(quantile).Should(BeIdenticalTo(1.2))
			})

			It("should return NaN for quantiles out of range [0.0, 1.0]", func() {
				sut.forTDigest()
				sut.AddSimpleDataSet()

				Expect(math.IsNaN(sut.Quantile(-0.01))).To(BeTrue())
				Expect(math.IsNaN(sut.Quantile(1.01))).To(BeTrue())
			})

			It("should cover full range of quantiles for percentiles in range [0.0, 1.0]", func() {
				sut.forTDigest()
				sut.AddSimpleDataSet()

				expectedValues := []struct{ percentile, value float64 }{
					{0, 1.2},
					{0.05, 1.2},
					{0.1, 1.2},
					{0.15, 1.2},
					{0.2, 1.2},
					{0.25, 1.2},
					{0.3, 1.35},
					{0.35, 1.66},
					{0.4, 1.98},
					{0.45, 2.04},
					{0.5, 2.11},
					{0.55, 2.19},
					{0.6, 2.25},
					{0.65, 2.33},
					{0.7, 2.4},
					{0.75, 2.47},
					{0.8, 2.54},
					{0.85, 2.62},
					{0.9, 2.68},
					{0.95, 2.75},
					{0.99, 2.81},
					{0.999, 2.97},
					{0.9999, 2.97},
					{1.0, 3.14},
				}

				for _, value := range expectedValues {
					Expect(sut.Quantile(value.percentile)).Should(
						BeNumerically("~", value.value, 0.01),
						fmt.Sprintf("at percentile %f value %f", value.percentile, value.value))
				}
			})
		})
	})

	Context("Centroids", func() {
		sut := centroidsSut{}

		It("has empty size for an empty centroids", func() {
			sut.forCentroids()
			Expect(sut.centroids.Size()).To(Equal(0))
		})

		It("should store centroid", func() {
			sut.forCentroids()
			sut.WithSingleCentroid()
			Expect(sut.centroids.Size()).To(Equal(1))
		})

		It("should store centroids ordered by mean", func() {
			sut.forCentroids()
			sut.WithDataset()

			list := sut.toList()
			Expect(list).To(Equal([]tdigest.Centroid{
				{Mean: 1.618, Weight: 3},
				{Mean: 2.718, Weight: 8},
				{Mean: 3.14, Weight: 15},
				{Mean: 4.765, Weight: 41},
				{Mean: 5.635, Weight: 31},
				{Mean: 6.123, Weight: 73},
				{Mean: 7.123, Weight: 41},
				{Mean: 8.156, Weight: 60},
				{Mean: 9.635, Weight: 98},
				{Mean: 10.635, Weight: 3},
			}))
		})

		It("computes total sum of all weights", func() {
			sut.forCentroids()
			sut.WithDataset()

			Expect(sut.centroids.TotalWeight()).To(Equal(uint64(373)))
		})

		It("do not finds index of centroid with cumulative sum for empty", func() {
			sut.forCentroids()

			sum, index, found := sut.centroidWithCumulativeSum(0)
			Expect(found).To(BeFalse())
			Expect(index).To(BeZero())
			Expect(sum).To(BeZero())
		})

		It("finds index 0 of centroid with 0 cumulative sum", func() {
			sut.forCentroids()
			sut.WithDataset()

			sum, index, found := sut.centroidWithCumulativeSum(0)
			Expect(found).To(BeTrue())
			Expect(index).To(BeZero())
			Expect(sum).To(Equal(uint64(3)))
		})

		It("finds index of centroid with cumulative sum", func() {
			sut.forCentroids()
			sut.WithDataset()

			sum, index, found := sut.centroidWithCumulativeSum(26)
			Expect(found).To(BeTrue())
			Expect(index).To(Equal(2))
			Expect(sum).To(Equal(uint64(26)))
		})

		It("finds last index of centroid with cumulative sum over total sum", func() {
			sut.forCentroids()
			sut.WithDataset()

			sum, index, found := sut.centroidWithCumulativeSum(1000000)
			Expect(found).To(BeTrue())
			Expect(index).To(Equal(9))
			Expect(sum).To(Equal(uint64(373)))
		})
	})
})

type centroidsSut struct {
	centroids *tdigest.Centroids
}

func (s *centroidsSut) forCentroids() {
	s.centroids = tdigest.NewCentroids(100)
}

func (s *centroidsSut) WithDataset() {
	s.centroids.AddCentroid(1.618, 3)
	s.centroids.AddCentroid(2.718, 8)
	s.centroids.AddCentroid(3.140, 15)
	s.centroids.AddCentroid(4.765, 41)
	s.centroids.AddCentroid(5.635, 31)
	s.centroids.AddCentroid(6.123, 73)
	s.centroids.AddCentroid(7.123, 41)
	s.centroids.AddCentroid(8.156, 60)
	s.centroids.AddCentroid(9.635, 98)
	s.centroids.AddCentroid(10.635, 3)
}

func (s *centroidsSut) toList() []tdigest.Centroid {
	return s.centroids.ToList()
}


func (s *centroidsSut) WithSingleCentroid() {
	s.centroids.AddCentroid(3.14, 5)
}


func (s *centroidsSut) centroidWithCumulativeSum(sum uint64) (uint64, int, bool) {
	return s.centroids.FittingCumulativeWeightCentroid(sum)
}

type tdigestSut struct {
	tdigest *tdigest.TDigest
}

func (t *tdigestSut) forTDigest() {
	t.tdigest = tdigest.NewTDigestWeightScaled(
		100,
		500,
	)
}

func (t *tdigestSut) forTDigestWithHihBuffer() {
	t.tdigest = tdigest.NewTDigestWeightScaled(
		100,
		10000,
	)
}

func (t *tdigestSut) AddEntry(mean float64) {
	t.tdigest.AddToBuffer(mean, 1)
}

func (t *tdigestSut) AddSimpleDataSet() {
	t.tdigest.AddToBuffer(1.2, 30)
	t.tdigest.AddToBuffer(1.98, 15)
	t.tdigest.AddToBuffer(2.81, 66)
	t.tdigest.AddToBuffer(3.14, 2)
}

func (t *tdigestSut) ToCentroids() []tdigest.Centroid {
	return t.tdigest.ToCentroids()
}

func (t *tdigestSut) AddRandomEntries(count int) {
	randomizer := rand.New(rand.NewPCG(190, 89992))
	for range count {
		t.AddEntry(0.5 + (10 * randomizer.Float64()))
	}
}

func (t *tdigestSut) AddDescreasingEntries(count int) {
	value := 1100.0
	decrement := 1000.0 / float64(count)
	for range count {
		t.AddEntry(value)
		value -= decrement
	}
}

func (t *tdigestSut) AddIncreasingEntries(count int) {
	value := 10.0
	increment := 1000.0 / float64(count)
	for range count {
		t.AddEntry(value)
		value += increment
	}
}

func (t *tdigestSut) Quantile(percentile float64) float64 {
	return t.tdigest.Quantile(percentile)
}

func round(value float64, decimals uint32) float64 {
	return math.Round(value*math.Pow(10, float64(decimals))) / math.Pow(10, float64(decimals))
}
