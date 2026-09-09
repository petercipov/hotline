package digestcompare_test

import (
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"text/tabwriter"

	ddhistogram "hotline/metrics/dd-latency-histogram"
	"hotline/metrics/radixtree"
	tdhistogram "hotline/metrics/td-latency-histogram"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The radix tree is generic over the digest, so the same window machinery can
// be driven with either implementation and the two compared on the axes that
// actually decide the choice: precision, memory, and whether the answer is
// reproducible when the work is partitioned differently.

var _ = Describe("Digest comparison", func() {
	sut := comparisonSut{}

	Context("precision over a realistic window", func() {
		It("keeps DDSketch inside the relative error guarantee", func() {
			sut.forLognormalWindow()

			for _, result := range sut.measureSketch() {
				Expect(result.relativeError).To(BeNumerically("<=", ddhistogram.Alpha),
					fmt.Sprintf("DDSketch q=%v estimated %.0f against exact %.0f",
						result.q, result.estimate, result.exact))
			}
		})

		It("reports where t-digest lands against the same oracle", func() {
			sut.forLognormalWindow()

			results := sut.measureTDigest()
			Expect(results).ToNot(BeEmpty())

			// t-digest gives no relative error guarantee on the value, so this
			// records what it actually does rather than asserting a bound
			sut.report("precision", sut.measureSketch(), results)
		})

		It("answers p0 and p100 exactly under both", func() {
			sut.forLognormalWindow()

			for _, results := range [][]quantileResult{sut.measureSketch(), sut.measureTDigest()} {
				for _, result := range results {
					if result.q <= 0.0 || result.q >= 1.0 {
						Expect(result.estimate).To(Equal(result.exact),
							fmt.Sprintf("q=%v must come from the exact scalars", result.q))
					}
				}
			}
		})
	})

	Context("memory over the same window", func() {
		It("holds the tree shape identical, so every byte of difference is payload", func() {
			sut.forLognormalWindow()

			_, sketchNodes := sut.sketchFootprint()
			_, digestNodes := sut.tdigestFootprint()

			// the tree shape is canonical: it depends only on which seconds
			// exist, never on the digest, so the node counts MUST match
			Expect(digestNodes).To(Equal(sketchNodes))
		})

		It("reports the retained footprint across t-digest tunings", func() {
			sut.forLognormalWindow()
			sut.reportMemory()
		})
	})

	Context("reproducibility under repartitioning", func() {
		It("gives DDSketch one distinct result across every shard layout", func() {
			sut.forLognormalWindow()

			Expect(sut.distinctSketchRepartitions()).To(Equal(1),
				"a repartition MUST NOT change a DDSketch")
		})

		It("shows t-digest changing with the shard layout", func() {
			sut.forLognormalWindow()

			distinct := sut.distinctTDigestRepartitions()
			sut.reportRepartition(1, distinct)

			// this is the property the design rests on, stated as a test so a
			// change in the t-digest merge cannot quietly claim to have it
			Expect(distinct).To(BeNumerically(">", 1),
				"t-digest merge re-clusters, so it cannot be order independent")
		})
	})
})

func targetQuantiles() []float64 {
	return []float64{0.0, 0.2, 0.5, 0.7, 0.99, 0.999, 1.0}
}

// exactNearestRank is the oracle: the q-quantile is the smallest value v such
// that at least ceil(q*n) values are <= v.
func exactNearestRank(values []int64, q float64) int64 {
	sorted := slices.Clone(values)
	slices.Sort(sorted)

	rank := int(math.Ceil(q * float64(len(sorted))))
	rank = max(rank, 1)
	rank = min(rank, len(sorted))
	return sorted[rank-1]
}

// digest is the constraint the comparison needs of either implementation: a
// tree payload that can also ingest measurements and answer quantiles. It lives
// here because the comparison is the only thing that treats the two digests
// uniformly — neither package needs to know the other exists.
type digest[T any] interface {
	radixtree.Mergeable[T]
	InsertLatency(latencyNS int64) error
	Quantiles(qs []float64) []float64
}

type quantileResult struct {
	q             float64
	exact         float64
	estimate      float64
	relativeError float64
}

type comparisonSut struct {
	// values carries every measurement in the window, second by second, so
	// both pipelines and the oracle see one identical stream
	seconds [][]int64
	all     []int64
	built   bool
	from    uint64
	to      uint64
}

const (
	comparisonSeconds      = 600
	comparisonEventsPerSec = 120
	comparisonMedianNS     = 40_000_000.0
	comparisonSigma        = 0.6
	comparisonTailFraction = 0.01
	comparisonTailMultiple = 25.0
	comparisonFirstSecond  = uint64(1_788_776_132)
)

// forLognormalWindow builds a lognormal latency stream with a multiplicative
// tail, which is the shape the parameter table was measured against.
func (s *comparisonSut) forLognormalWindow() {
	if s.built {
		return // the dataset is deterministic, so build it once
	}
	s.built = true

	randomizer := rand.New(rand.NewPCG(0x5eed, 0xf00d))
	mu := math.Log(comparisonMedianNS)

	s.seconds = make([][]int64, comparisonSeconds)
	s.from = comparisonFirstSecond
	s.to = comparisonFirstSecond + comparisonSeconds - 1

	for offset := range comparisonSeconds {
		for range comparisonEventsPerSec {
			latency := math.Exp(mu + comparisonSigma*randomizer.NormFloat64())
			if randomizer.Float64() < comparisonTailFraction {
				latency *= comparisonTailMultiple
			}

			ns := int64(latency)
			s.seconds[offset] = append(s.seconds[offset], ns)
			s.all = append(s.all, ns)
		}
	}
}

// feed replays the identical stream into a tree of the given digest type.
func replay[T digest[T]](s *comparisonSut, empty radixtree.Factory[T]) *radixtree.Tree[T] {
	tree := radixtree.New(empty)
	for offset, values := range s.seconds {
		digest := empty()
		for _, ns := range values {
			Expect(digest.InsertLatency(ns)).To(Succeed())
		}
		tree.Update(comparisonFirstSecond+uint64(offset), digest)
	}
	return tree
}

func measure[T digest[T]](s *comparisonSut, empty radixtree.Factory[T]) []quantileResult {
	window := replay(s, empty).AggregateRange(s.from, s.to)
	estimates := window.Quantiles(targetQuantiles())

	results := make([]quantileResult, 0, len(estimates))
	for i, q := range targetQuantiles() {
		exact := float64(exactNearestRank(s.all, q))
		results = append(results, quantileResult{
			q:             q,
			exact:         exact,
			estimate:      estimates[i],
			relativeError: math.Abs(estimates[i]-exact) / exact,
		})
	}
	return results
}

func (s *comparisonSut) measureSketch() []quantileResult {
	return measure(s, ddhistogram.NewSketch)
}

func (s *comparisonSut) measureTDigest() []quantileResult {
	return measure(s, tdhistogram.NewSketch)
}

func (s *comparisonSut) sketchFootprint() (int, int) {
	tree := replay(s, ddhistogram.NewSketch)
	return tree.SizeInBytes(), tree.NodeCount()
}

func (s *comparisonSut) tdigestFootprint() (int, int) {
	tree := replay(s, tdhistogram.NewSketch)
	return tree.SizeInBytes(), tree.NodeCount()
}

// distinctRepartitions splits one second of measurements into k partitions,
// merges them in a random order, and counts how many distinct digests the
// different layouts produced. For a commutative monoid the answer is 1.
func distinctRepartitions[T digest[T]](s *comparisonSut, empty radixtree.Factory[T], trials int) int {
	randomizer := rand.New(rand.NewPCG(0xa11ce, 0xb0b))
	values := s.seconds[0]

	var distinct []T
	for range trials {
		k := 2 + randomizer.IntN(8)
		partitions := make([]T, k)
		for i := range partitions {
			partitions[i] = empty()
		}
		for _, ns := range values {
			Expect(partitions[randomizer.IntN(k)].InsertLatency(ns)).To(Succeed())
		}

		merged := empty()
		for _, index := range randomizer.Perm(k) {
			merged.Merge(partitions[index])
		}

		seen := false
		for _, existing := range distinct {
			if existing.Equal(merged) {
				seen = true
				break
			}
		}
		if !seen {
			distinct = append(distinct, merged)
		}
	}
	return len(distinct)
}

func (s *comparisonSut) distinctSketchRepartitions() int {
	return distinctRepartitions(s, ddhistogram.NewSketch, 200)
}

func (s *comparisonSut) distinctTDigestRepartitions() int {
	return distinctRepartitions(s, tdhistogram.NewSketch, 200)
}

func (s *comparisonSut) report(title string, sketch, digest []quantileResult) {
	out := tabwriter.NewWriter(GinkgoWriter, 0, 0, 2, ' ', 0)
	fmt.Fprintf(out, "\n%s over %d seconds x %d events\n", title, comparisonSeconds, comparisonEventsPerSec)
	fmt.Fprintln(out, "q\texact (ms)\tDDSketch (ms)\terr\tt-digest (ms)\terr")
	for i := range sketch {
		fmt.Fprintf(out, "%.3f\t%.2f\t%.2f\t%.4f\t%.2f\t%.4f\n",
			sketch[i].q,
			sketch[i].exact/1e6,
			sketch[i].estimate/1e6, sketch[i].relativeError,
			digest[i].estimate/1e6, digest[i].relativeError)
	}
	_ = out.Flush()
}

// reportMemory sweeps t-digest tunings alongside the sketch. The insert buffer
// is charged to every node in the tree, so at the default 500 it dominates the
// centroids completely — the sweep is what separates the intrinsic cost of a
// digest from the cost of how it happens to be tuned.
func (s *comparisonSut) reportMemory() {
	type row struct {
		label   string
		bytes   int
		nodes   int
		tailErr float64
	}

	sketchBytes, sketchNodes := s.sketchFootprint()
	rows := []row{{"DDSketch (alpha=0.02)", sketchBytes, sketchNodes, tailError(s.measureSketch())}}

	for _, tuning := range []struct {
		compression, buffer int
	}{{100, 500}, {100, 32}, {50, 32}} {
		empty := tdhistogram.NewSketchSized(tuning.compression, tuning.buffer)
		tree := replay(s, empty)
		rows = append(rows, row{
			label:   fmt.Sprintf("t-digest (d=%d, buf=%d)", tuning.compression, tuning.buffer),
			bytes:   tree.SizeInBytes(),
			nodes:   tree.NodeCount(),
			tailErr: tailError(measure(s, empty)),
		})
	}

	out := tabwriter.NewWriter(GinkgoWriter, 0, 0, 2, ' ', 0)
	fmt.Fprintf(out, "\nmemory over %d seconds x %d events\n", comparisonSeconds, comparisonEventsPerSec)
	fmt.Fprintln(out, "digest\tnodes\ttotal\tper node\tp99.9 err")
	for _, r := range rows {
		fmt.Fprintf(out, "%s\t%d\t%.1f KB\t%d B\t%.4f\n",
			r.label, r.nodes, float64(r.bytes)/1024, r.bytes/r.nodes, r.tailErr)
	}
	_ = out.Flush()
}

// tailError picks the deepest interior quantile, which is where repeated
// re-clustering through the tree shows up first.
func tailError(results []quantileResult) float64 {
	for _, result := range results {
		if result.q == 0.999 {
			return result.relativeError
		}
	}
	return math.NaN()
}

func (s *comparisonSut) reportRepartition(sketch, digest int) {
	fmt.Fprintf(GinkgoWriter,
		"\nrepartition determinism over 200 trials, k in [2,9]\n  DDSketch distinct results: %d\n  t-digest distinct results: %d\n",
		sketch, digest)
}
