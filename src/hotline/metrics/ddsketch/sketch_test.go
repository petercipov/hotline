package ddsketch_test

import (
	"fmt"
	"hotline/metrics/ddsketch"
	"math"
	"math/rand/v2"
	"sort"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// normativeDataset is the dataset of spec 9.3.
func normativeDataset() []uint64 {
	return []uint64{
		1200000, 3400000, 5100000, 7800000, 9900000, 12500000, 15000000, 18200000,
		21000000, 24400000, 28000000, 33000000, 39000000, 46000000, 55000000,
		68000000, 84000000, 110000000, 190000000, 920000000,
	}
}

var _ = Describe("Sketch", func() {
	sut := sketchSut{}

	Context("mapping constants (spec 9.1)", func() {
		It("derives gamma and ln(gamma) from alpha", func() {
			Expect(ddsketch.Gamma).To(Equal(1.0408163265306123))
			Expect(ddsketch.LnGamma).To(Equal(0.040005334613699206))
		})

	})

	Context("index vectors (spec 9.2)", func() {
		vectors := []struct {
			latency       uint64
			index         int32
			reconstructed float64
		}{
			{1_000, 173, 993.0},
			{100_000, 288, 98847.5},
			{1_000_000, 346, 1006151.4},
			{2_500_000, 369, 2525035.7},
			{10_000_000, 403, 9839811.8},
			{40_000_000, 438, 39909855.4},
			{250_000_000, 484, 251355604.6},
			{1_000_000_000, 519, 1019487571.6},
			{30_000_000_000, 604, 30561882496.3},
			{100_000_000_000, 634, 101485263507.3},
		}

		for _, v := range vectors {
			It(fmt.Sprintf("maps %d ns to bucket %d", v.latency, v.index), func() {
				Expect(ddsketch.Index(v.latency)).To(Equal(v.index))
				Expect(ddsketch.BucketValue(v.index)).To(BeNumerically("~", v.reconstructed, 0.05))
			})

			It(fmt.Sprintf("keeps %d ns within alpha of its bucket representative", v.latency), func() {
				reconstructed := ddsketch.BucketValue(ddsketch.Index(v.latency))
				Expect(math.Abs(reconstructed-float64(v.latency)) / float64(v.latency)).
					To(BeNumerically("<=", ddsketch.Alpha))
			})
		}

		It("keeps both edges of bucket 438 inside bucket 438", func() {
			Expect(ddsketch.Index(39127310)).To(Equal(int32(438)))
			Expect(ddsketch.Index(40724342)).To(Equal(int32(438)))
		})
	})

	Context("insert", func() {
		It("starts empty with the identity scalars", func() {
			sut.forEmptySketch()
			Expect(sut.sketch.Count()).To(BeZero())
			Expect(sut.sketch.Zeros()).To(BeZero())
			Expect(sut.sketch.Sum()).To(BeZero())
			Expect(sut.sketch.Min()).To(Equal(uint64(math.MaxUint64)))
			Expect(sut.sketch.Max()).To(BeZero())
			Expect(sut.sketch.Buckets()).To(BeEmpty())
		})

		It("accumulates the exact scalars and buckets of the normative dataset (spec 9.3)", func() {
			sut.forNormativeDataset()

			Expect(sut.sketch.Count()).To(Equal(uint64(20)))
			Expect(sut.sketch.Zeros()).To(BeZero())
			Expect(sut.sketch.Min()).To(Equal(uint64(1200000)))
			Expect(sut.sketch.Max()).To(Equal(uint64(920000000)))
			Expect(sut.sketch.Sum()).To(Equal(uint64(1691500000)))
			Expect(sut.sketch.Buckets()).To(Equal(map[int32]uint64{
				350: 1, 376: 1, 387: 1, 397: 1, 403: 1, 409: 1, 414: 1, 418: 1, 422: 1, 426: 1,
				429: 1, 433: 1, 437: 1, 442: 1, 446: 1, 451: 1, 457: 1, 463: 1, 477: 1, 516: 1,
			}))
		})

		It("counts values below MIN_NS as zeros, not as a bucket", func() {
			sut.forEmptySketch()
			sut.Insert(999)
			sut.Insert(1)

			Expect(sut.sketch.Zeros()).To(Equal(uint64(2)))
			Expect(sut.sketch.Underflow()).To(Equal(uint64(2)))
			Expect(sut.sketch.Buckets()).To(BeEmpty())
			Expect(sut.sketch.Count()).To(Equal(uint64(2)))
			Expect(sut.sketch.Min()).To(Equal(uint64(1)))
		})

		It("clamps values above the ceiling into the top bucket and counts the overflow", func() {
			sut.forEmptySketch()
			ceiling := ddsketch.DefaultRange().MaxNS
			sut.Insert(ceiling * 10)

			Expect(sut.sketch.Buckets()).To(Equal(map[int32]uint64{ddsketch.Index(ceiling): 1}))
			Expect(sut.sketch.Overflow()).To(Equal(uint64(1)))
			Expect(sut.sketch.Max()).To(Equal(ceiling * 10))
		})

		It("keeps count equal to zeros plus bucket counts", func() {
			sut.forRandomSketch(1, 5000)
			Expect(sut.sketch.Count()).To(Equal(sut.sketch.Zeros() + sut.bucketTotal()))
		})
	})

	Context("quantiles", func() {
		It("is undefined for an empty sketch", func() {
			sut.forEmptySketch()
			for _, q := range sut.Quantiles(0.0, 0.5, 1.0) {
				Expect(math.IsNaN(q)).To(BeTrue())
			}
		})

		It("reproduces the normative quantiles (spec 9.3)", func() {
			sut.forNormativeDataset()

			expected := []struct{ q, value float64 }{
				{0.0, 1200000.0},
				{0.20, 7740022.4},
				{0.50, 24693974.8},
				{0.70, 46835648.5},
				{0.90, 108500703.9},
				{0.99, 904189891.6},
				{0.999, 904189891.6},
				{1.0, 920000000.0},
			}

			qs := make([]float64, 0, len(expected))
			for _, e := range expected {
				qs = append(qs, e.q)
			}
			got := sut.Quantiles(qs...)

			for i, e := range expected {
				Expect(got[i]).To(BeNumerically("~", e.value, 0.05),
					fmt.Sprintf("at q %v", e.q))
			}
		})

		It("answers p0 and p100 exactly from min and max", func() {
			sut.forNormativeDataset()
			got := sut.Quantiles(0.0, 1.0)

			Expect(got[0]).To(Equal(float64(1200000)))
			Expect(got[1]).To(Equal(float64(920000000)))
		})

		It("stays within alpha of the exact quantile across distributions (spec 8.2)", func() {
			for _, distribution := range []string{"lognormal", "bimodal", "uniform", "heavytail", "identical"} {
				values := generateDistribution(distribution, 20_000)
				sut.forValues(values)

				qs := []float64{0.0, 0.2, 0.5, 0.7, 0.99, 0.999, 1.0}
				got := sut.Quantiles(qs...)

				for i, q := range qs {
					exact := float64(exactNearestRank(values, q))
					Expect(math.Abs(got[i]-exact)).To(BeNumerically("<=", ddsketch.Alpha*exact),
						fmt.Sprintf("%s at q %v: got %v exact %v", distribution, q, got[i], exact))
				}
			}
		})

		It("returns the single value for a one-element sketch", func() {
			sut.forValues([]uint64{42_000_000})
			got := sut.Quantiles(0.0, 0.5, 1.0)

			Expect(got[0]).To(Equal(float64(42_000_000)))
			Expect(got[1]).To(BeNumerically("~", 42_000_000.0, ddsketch.Alpha*42_000_000))
			Expect(got[2]).To(Equal(float64(42_000_000)))
		})

		It("clamps quantiles outside [0, 1] to the exact extremes", func() {
			sut.forNormativeDataset()
			got := sut.Quantiles(-0.5, 1.5)

			Expect(got[0]).To(Equal(float64(1200000)))
			Expect(got[1]).To(Equal(float64(920000000)))
		})

		It("returns zero for quantiles that fall inside the zeros bucket", func() {
			sut.forValues([]uint64{1, 2, 3, 500_000_000})
			got := sut.Quantiles(0.5, 1.0)

			Expect(got[0]).To(BeZero())
			Expect(got[1]).To(Equal(float64(500_000_000)))
		})
	})

	Context("merge (spec 8.1)", func() {
		It("has the empty sketch as a two sided identity", func() {
			sut.forNormativeDataset()
			original := sut.sketch.Clone()

			left := ddsketch.NewSketch()
			left.Merge(sut.sketch)
			right := sut.sketch.Clone()
			right.Merge(ddsketch.NewSketch())

			Expect(left.Equal(original)).To(BeTrue())
			Expect(right.Equal(original)).To(BeTrue())
		})

		It("is commutative bit exactly", func() {
			for seed := range uint64(50) {
				a := randomSketch(seed, 200)
				b := randomSketch(seed+1000, 200)

				ab := a.Clone()
				ab.Merge(b)
				ba := b.Clone()
				ba.Merge(a)

				Expect(ab.Equal(ba)).To(BeTrue(), fmt.Sprintf("seed %d", seed))
			}
		})

		It("is associative bit exactly", func() {
			for seed := range uint64(50) {
				a := randomSketch(seed, 200)
				b := randomSketch(seed+1000, 200)
				c := randomSketch(seed+2000, 200)

				left := a.Clone()
				left.Merge(b)
				left.Merge(c)

				bc := b.Clone()
				bc.Merge(c)
				right := a.Clone()
				right.Merge(bc)

				Expect(left.Equal(right)).To(BeTrue(), fmt.Sprintf("seed %d", seed))
			}
		})

		It("loses nothing at any merge depth", func() {
			values := generateDistribution("lognormal", 5000)

			single := ddsketch.NewSketch()
			for _, v := range values {
				single.Insert(v)
			}

			Expect(mergeTree(values, 7).Equal(single)).To(BeTrue())
		})
	})

	Context("repartition determinism (spec 8.3 / 9.4)", func() {
		It("yields exactly one distinct result over 200 random repartitions", func() {
			single := ddsketch.NewSketch()
			for _, v := range normativeDataset() {
				single.Insert(v)
			}

			randomizer := rand.New(rand.NewPCG(9, 4))
			for trial := range 200 {
				k := 2 + randomizer.IntN(8)
				merged := randomPartitionMerge(normativeDataset(), k, randomizer)
				Expect(merged.Equal(single)).To(BeTrue(), fmt.Sprintf("trial %d with k=%d", trial, k))
			}
		})

		It("survives repartitioning of a large mixed dataset", func() {
			values := generateDistribution("heavytail", 20_000)

			single := ddsketch.NewSketch()
			for _, v := range values {
				single.Insert(v)
			}

			randomizer := rand.New(rand.NewPCG(77, 13))
			for trial := range 50 {
				k := 2 + randomizer.IntN(15)
				merged := randomPartitionMerge(values, k, randomizer)
				Expect(merged.Equal(single)).To(BeTrue(), fmt.Sprintf("trial %d with k=%d", trial, k))
			}
		})
	})

	Context("edge cases (spec 8.6)", func() {
		It("keeps values sitting exactly on a bucket boundary in one bucket", func() {
			boundary := uint64(math.Pow(ddsketch.Gamma, 438))
			Expect(ddsketch.Index(boundary)).To(BeNumerically("~", 438, 1))
		})

		It("puts identical values into a single bucket", func() {
			sut.forValues([]uint64{7_000_000, 7_000_000, 7_000_000, 7_000_000})

			Expect(sut.sketch.Buckets()).To(HaveLen(1))
			Expect(sut.Quantiles(0.5)[0]).To(BeNumerically("~", 7_000_000.0, ddsketch.Alpha*7_000_000))
		})

		It("never stores a zero count bucket", func() {
			sut.forRandomSketch(3, 1000)
			other := randomSketch(4, 1000)
			sut.sketch.Merge(other)

			for _, count := range sut.sketch.Buckets() {
				Expect(count).ToNot(BeZero())
			}
		})
	})
})

var _ = Describe("Sketch.SizeInBytes", func() {
	sut := sketchSut{}

	It("is non zero for an empty sketch, which still carries the fixed scalars", func() {
		sut.forEmptySketch()
		Expect(sut.sketch.SizeInBytes()).To(BeNumerically(">", 0))
	})

	It("grows with every newly populated bucket", func() {
		// the premise: these land in different buckets
		Expect(ddsketch.Index(1_000_000)).ToNot(Equal(ddsketch.Index(2_000_000)))

		sut.forEmptySketch()
		empty := sut.sketch.SizeInBytes()

		sut.Insert(1_000_000)
		one := sut.sketch.SizeInBytes()

		sut.Insert(2_000_000)
		two := sut.sketch.SizeInBytes()

		Expect(one).To(BeNumerically(">", empty))
		Expect(two).To(BeNumerically(">", one))
	})

	It("does not grow when a value lands in a bucket that already exists", func() {
		// the premise: these share a bucket
		Expect(ddsketch.Index(1_000_000)).To(Equal(ddsketch.Index(1_020_000)))

		sut.forValues([]uint64{1_000_000})
		before := sut.sketch.SizeInBytes()

		sut.Insert(1_020_000)

		Expect(sut.sketch.Count()).To(Equal(uint64(2)))
		Expect(sut.sketch.SizeInBytes()).To(Equal(before))
	})

	It("does not grow for a value below the range, which is counted as a zero", func() {
		sut.forEmptySketch()
		empty := sut.sketch.SizeInBytes()

		sut.Insert(ddsketch.DefaultRange().MinNS - 1)

		Expect(sut.sketch.Zeros()).To(Equal(uint64(1)))
		Expect(sut.sketch.SizeInBytes()).To(Equal(empty))
	})

	It("costs one bucket however many values sit above the range, since they all clamp", func() {
		maxNS := ddsketch.DefaultRange().MaxNS

		sut.forValues([]uint64{maxNS + 1})
		clamped := sut.sketch.SizeInBytes()

		sut.Insert(maxNS * 2)
		sut.Insert(maxNS * 3)

		Expect(sut.sketch.Overflow()).To(Equal(uint64(3)))
		Expect(sut.sketch.SizeInBytes()).To(Equal(clamped))
	})

	It("is unchanged by a clone, which copies the same buckets", func() {
		sut.forNormativeDataset()

		Expect(sut.sketch.Clone().SizeInBytes()).To(Equal(sut.sketch.SizeInBytes()))
	})

	It("reports the union of the buckets after a merge", func() {
		low := ddsketch.NewSketch()
		low.Insert(1_000_000)
		high := ddsketch.NewSketch()
		high.Insert(8_000_000)

		before := low.SizeInBytes()
		low.Merge(high)
		grown := low.SizeInBytes()
		Expect(grown).To(BeNumerically(">", before))

		// folding the same buckets in again adds no entries
		low.Merge(high)
		Expect(low.SizeInBytes()).To(Equal(grown))
	})

	It("depends on the buckets populated, not on the span the sketch resolves", func() {
		wide := ddsketch.NewSketchInRange(ddsketch.Range{MinNS: 1, MaxNS: 1 << 40})
		narrow := ddsketch.NewSketchInRange(ddsketch.Range{MinNS: 1_000_000, MaxNS: 2_000_000})

		wide.Insert(1_500_000)
		narrow.Insert(1_500_000)

		Expect(wide.SizeInBytes()).To(Equal(narrow.SizeInBytes()))
	})
})

type sketchSut struct {
	sketch *ddsketch.Sketch
}

func (s *sketchSut) forEmptySketch() {
	s.sketch = ddsketch.NewSketch()
}

func (s *sketchSut) forValues(values []uint64) {
	s.sketch = ddsketch.NewSketch()
	for _, v := range values {
		s.sketch.Insert(v)
	}
}

func (s *sketchSut) forNormativeDataset() {
	s.forValues(normativeDataset())
}

func (s *sketchSut) forRandomSketch(seed uint64, count int) {
	s.sketch = randomSketch(seed, count)
}

func (s *sketchSut) Insert(v uint64) {
	s.sketch.Insert(v)
}

func (s *sketchSut) Quantiles(qs ...float64) []float64 {
	return s.sketch.Quantiles(qs)
}

func (s *sketchSut) bucketTotal() uint64 {
	total := uint64(0)
	for _, c := range s.sketch.Buckets() {
		total += c
	}
	return total
}

func randomSketch(seed uint64, count int) *ddsketch.Sketch {
	randomizer := rand.New(rand.NewPCG(seed, seed*7+1))
	s := ddsketch.NewSketch()
	r := ddsketch.DefaultRange()
	for i := range count {
		// deliberately spans all three destinations: a uniform draw over the
		// whole span would essentially never land below the 1 ms floor
		switch i % 8 {
		case 0:
			s.Insert(randomizer.Uint64N(r.MinNS))
		case 7:
			s.Insert(r.MaxNS + randomizer.Uint64N(r.MaxNS))
		default:
			s.Insert(r.MinNS + randomizer.Uint64N(r.MaxNS-r.MinNS))
		}
	}
	return s
}

func mergeTree(values []uint64, fanout int) *ddsketch.Sketch {
	if len(values) <= fanout {
		s := ddsketch.NewSketch()
		for _, v := range values {
			s.Insert(v)
		}
		return s
	}
	chunk := (len(values) + fanout - 1) / fanout
	acc := ddsketch.NewSketch()
	for start := 0; start < len(values); start += chunk {
		end := min(start+chunk, len(values))
		acc.Merge(mergeTree(values[start:end], fanout))
	}
	return acc
}

func randomPartitionMerge(values []uint64, k int, randomizer *rand.Rand) *ddsketch.Sketch {
	partitions := make([]*ddsketch.Sketch, k)
	for i := range partitions {
		partitions[i] = ddsketch.NewSketch()
	}
	for _, v := range values {
		partitions[randomizer.IntN(k)].Insert(v)
	}
	randomizer.Shuffle(k, func(i, j int) {
		partitions[i], partitions[j] = partitions[j], partitions[i]
	})

	acc := ddsketch.NewSketch()
	for _, p := range partitions {
		acc.Merge(p)
	}
	return acc
}

func generateDistribution(name string, count int) []uint64 {
	randomizer := rand.New(rand.NewPCG(uint64(len(name)), 4242))
	values := make([]uint64, 0, count)
	for i := range count {
		switch name {
		case "lognormal":
			values = append(values, uint64(math.Exp(math.Log(40_000_000)+0.6*randomizer.NormFloat64())))
		case "bimodal":
			if i%3 == 0 {
				values = append(values, 2_000_000+randomizer.Uint64N(500_000))
			} else {
				values = append(values, 300_000_000+randomizer.Uint64N(50_000_000))
			}
		case "uniform":
			values = append(values, 1_000_000+randomizer.Uint64N(900_000_000))
		case "heavytail":
			if i%100 == 0 {
				values = append(values, 1_000_000_000+randomizer.Uint64N(9_000_000_000))
			} else {
				values = append(values, 1_000_000+randomizer.Uint64N(20_000_000))
			}
		case "identical":
			values = append(values, 33_000_000)
		}
	}
	return values
}

// exactNearestRank is the oracle for spec 3.5: the smallest value v such that at
// least ceil(q*n) values are <= v.
func exactNearestRank[T int64 | uint64](values []T, q float64) T {
	sorted := make([]T, len(values))
	copy(sorted, values)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	rank := int(math.Ceil(q * float64(len(sorted))))
	if rank < 1 {
		rank = 1
	}
	if rank > len(sorted) {
		rank = len(sorted)
	}
	return sorted[rank-1]
}

var _ = Describe("Range", func() {
	sut := sketchSut{}

	Context("the default", func() {
		It("spans 1 ms to 15 minutes", func() {
			r := ddsketch.DefaultRange()

			Expect(r.MinNS).To(Equal(uint64(1_000_000)))
			Expect(r.MaxNS).To(Equal(uint64(900_000_000_000)))
		})

		It("resolves that span into 344 buckets", func() {
			r := ddsketch.DefaultRange()

			Expect(ddsketch.Index(r.MinNS)).To(Equal(int32(346)))
			Expect(ddsketch.Index(r.MaxNS)).To(Equal(int32(689)))
			Expect(r.Buckets()).To(Equal(int32(344)))
		})

		It("counts a sub millisecond measurement as a zero", func() {
			sut.forEmptySketch()
			sut.Insert(500_000)

			Expect(sut.sketch.Zeros()).To(Equal(uint64(1)))
			Expect(sut.sketch.Underflow()).To(Equal(uint64(1)))
			Expect(sut.sketch.Buckets()).To(BeEmpty())
			Expect(sut.sketch.Min()).To(Equal(uint64(500_000)))
		})

		It("clamps an hour into the top bucket", func() {
			sut.forEmptySketch()
			sut.Insert(3_600_000_000_000)

			Expect(sut.sketch.Buckets()).To(Equal(map[int32]uint64{689: 1}))
			Expect(sut.sketch.Overflow()).To(Equal(uint64(1)))
			Expect(sut.sketch.Max()).To(Equal(uint64(3_600_000_000_000)))
		})
	})

	Context("a configured range", func() {
		It("resolves values the default would count as zeros", func() {
			microseconds := ddsketch.Range{MinNS: 1_000, MaxNS: 100_000_000_000}
			sketch := ddsketch.NewSketchInRange(microseconds)

			sketch.Insert(500_000)

			Expect(sketch.Zeros()).To(BeZero())
			Expect(sketch.Buckets()).To(Equal(map[int32]uint64{
				ddsketch.Index(500_000): 1,
			}))
		})

		It("clamps at its own ceiling, not the default one", func() {
			narrow := ddsketch.Range{MinNS: 1_000_000, MaxNS: 10_000_000_000}
			sketch := ddsketch.NewSketchInRange(narrow)

			sketch.Insert(100_000_000_000)

			Expect(sketch.Overflow()).To(Equal(uint64(1)))
			Expect(sketch.Buckets()).To(Equal(map[int32]uint64{
				ddsketch.Index(10_000_000_000): 1,
			}))
		})

		It("travels with the sketch", func() {
			narrow := ddsketch.Range{MinNS: 1_000_000, MaxNS: 10_000_000_000}

			Expect(ddsketch.NewSketchInRange(narrow).Range()).To(Equal(narrow))
			Expect(ddsketch.NewSketch().Range()).To(Equal(ddsketch.DefaultRange()))
		})

		It("survives a clone", func() {
			narrow := ddsketch.Range{MinNS: 1_000_000, MaxNS: 10_000_000_000}
			sketch := ddsketch.NewSketchInRange(narrow)

			Expect(sketch.Clone().Range()).To(Equal(narrow))
		})
	})

	Context("validation", func() {
		It("rejects a zero lower bound, which has no logarithm", func() {
			err := ddsketch.Range{MinNS: 0, MaxNS: 1_000_000}.Validate()
			Expect(err).To(MatchError(ddsketch.ErrInvalidRange))
		})

		It("rejects bounds that do not ascend", func() {
			Expect(ddsketch.Range{MinNS: 1_000_000, MaxNS: 1_000_000}.Validate()).
				To(MatchError(ddsketch.ErrInvalidRange))
			Expect(ddsketch.Range{MinNS: 1_000_000, MaxNS: 1_000}.Validate()).
				To(MatchError(ddsketch.ErrInvalidRange))
		})

		It("accepts the default", func() {
			Expect(ddsketch.DefaultRange().Validate()).To(Succeed())
		})
	})

	Context("mixing ranges", func() {
		// a bucket index means the same thing under any range, but zeros and
		// the clamp do not, so folding two ranges together is silent nonsense
		It("is distinguished by Equal", func() {
			narrow := ddsketch.Range{MinNS: 1_000_000, MaxNS: 10_000_000_000}

			left := ddsketch.NewSketch()
			right := ddsketch.NewSketchInRange(narrow)

			Expect(left.Equal(right)).To(BeFalse())
		})

		It("is rejected by AddPartial rather than silently folded", func() {
			narrow := ddsketch.Range{MinNS: 1_000_000, MaxNS: 10_000_000_000}
			sliding, err := ddsketch.NewSlidingWindowSketch(ddsketch.DefaultWindow, 10*time.Minute)
			Expect(err).ToNot(HaveOccurred())

			foreign := ddsketch.NewSketchInRange(narrow)
			foreign.Insert(5_000_000)

			Expect(sliding.AddPartial(1000, foreign)).To(MatchError(ddsketch.ErrRangeMismatch))
		})

		It("accepts a partial built on the same range", func() {
			partial := ddsketch.NewSketch()
			partial.Insert(5_000_000)
			sliding, err := ddsketch.NewSlidingWindowSketch(ddsketch.DefaultWindow, 10*time.Minute)
			Expect(err).ToNot(HaveOccurred())

			Expect(sliding.AddPartial(1000, partial)).To(Succeed())
			Expect(sliding.Window(1001).Count()).To(Equal(uint64(1)))
		})
	})

	Context("a sliding window sketch over a configured range", func() {
		It("gives every sketch in the tree that range", func() {
			microseconds := ddsketch.Range{MinNS: 1_000, MaxNS: 100_000_000_000}
			sliding, err := ddsketch.NewSlidingWindowSketchInRange(ddsketch.DefaultWindow, 10*time.Minute, microseconds)
			Expect(err).ToNot(HaveOccurred())

			Expect(sliding.Add(1000*ddsketch.NanosPerSecond, 500_000)).To(Succeed())

			window := sliding.Window(1001)
			Expect(window.Range()).To(Equal(microseconds))
			Expect(window.Zeros()).To(BeZero())
			Expect(window.Count()).To(Equal(uint64(1)))
			Expect(sliding.Tree().CheckInvariants()).To(Succeed())
		})

		It("refuses an invalid range", func() {
			_, err := ddsketch.NewSlidingWindowSketchInRange(ddsketch.DefaultWindow, 10*time.Minute, ddsketch.Range{MinNS: 0, MaxNS: 0})
			Expect(err).To(MatchError(ddsketch.ErrInvalidRange))
		})
	})
})
