package tdlatencyhistogram_test

import (
	"math"

	tdhistogram "hotline/metrics/td-latency-histogram"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func targetQuantiles() []float64 {
	return []float64{0.0, 0.2, 0.5, 0.7, 0.99, 0.999, 1.0}
}

var _ = Describe("Sketch", func() {
	sut := sketchSut{}

	Context("the Digest contract", func() {
		It("rejects a negative latency loudly", func() {
			sut.forEmpty()
			Expect(sut.digest.InsertLatency(-1)).To(MatchError(tdhistogram.ErrNegativeLatency))
		})

		It("yields NaN for every quantile when empty", func() {
			sut.forEmpty()
			for _, q := range sut.digest.Quantiles(targetQuantiles()) {
				Expect(math.IsNaN(q)).To(BeTrue())
			}
		})

		It("carries exact count, sum, min and max alongside", func() {
			sut.forValues(5_000_000, 1_000_000, 9_000_000)

			Expect(sut.digest.Count()).To(Equal(uint64(3)))
			Expect(sut.digest.Sum()).To(Equal(uint64(15_000_000)))
			Expect(sut.digest.Min()).To(Equal(uint64(1_000_000)))
			Expect(sut.digest.Max()).To(Equal(uint64(9_000_000)))
		})

		It("answers p0 and p100 from those exact scalars", func() {
			sut.forValues(5_000_000, 1_000_000, 9_000_000)

			extremes := sut.digest.Quantiles([]float64{0.0, 1.0})
			Expect(extremes[0]).To(Equal(1_000_000.0))
			Expect(extremes[1]).To(Equal(9_000_000.0))
		})

		It("is its own two sided identity when merging an empty digest", func() {
			sut.forValues(5_000_000, 1_000_000, 9_000_000)
			before := sut.digest.Clone()

			sut.digest.Merge(tdhistogram.NewSketch())

			Expect(sut.digest.Equal(before)).To(BeTrue())
		})
	})

	Context("Clone", func() {
		// Seal hands the tree a clone of the still open digest, so an aliasing
		// Clone would let a later correction mutate an already sealed leaf
		It("produces an equal but independent copy", func() {
			sut.forValues(5_000_000, 1_000_000, 9_000_000)
			clone := sut.digest.Clone()
			Expect(clone.Equal(sut.digest)).To(BeTrue())

			Expect(clone.InsertLatency(700_000_000)).To(Succeed())

			Expect(clone.Equal(sut.digest)).To(BeFalse())
			Expect(sut.digest.Count()).To(Equal(uint64(3)))
			Expect(sut.digest.Max()).To(Equal(uint64(9_000_000)))
		})

		It("copies centroids still sitting unprocessed in the buffer", func() {
			sut.forValues(5_000_000, 1_000_000, 9_000_000)

			// nothing has forced a compression yet, so this only holds if the
			// clone carried the buffer across
			Expect(sut.digest.Clone().Centroids()).To(Equal(sut.digest.Centroids()))
		})
	})

})

type sketchSut struct {
	digest *tdhistogram.Sketch
}

func (s *sketchSut) forEmpty() {
	s.digest = tdhistogram.NewSketch()
}

func (s *sketchSut) forValues(valuesNS ...int64) {
	s.forEmpty()
	for _, ns := range valuesNS {
		Expect(s.digest.InsertLatency(ns)).To(Succeed())
	}
}
