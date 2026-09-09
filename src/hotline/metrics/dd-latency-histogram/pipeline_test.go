package ddlatencyhistogram_test

import (
	"fmt"
	ddhistogram "hotline/metrics/dd-latency-histogram"
	"math"
	"math/rand/v2"
	"sort"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// anchorSec is the epoch second the normative prefix vectors are anchored on.
const anchorSec = uint64(1788776132)

func targetQuantiles() []float64 {
	return []float64{0.0, 0.2, 0.5, 0.7, 0.99, 0.999, 1.0}
}

var _ = Describe("Pipeline", func() {
	sut := pipelineSut{}

	Context("ingestion", func() {
		It("rejects a negative latency loudly", func() {
			sut.forPipeline(600)

			err := sut.pipeline.Add(anchorSec*ddhistogram.NanosPerSecond, -1)
			Expect(err).To(MatchError(ddhistogram.ErrNegativeLatency))
		})

		It("collapses any number of events in a second into one leaf", func() {
			sut.forPipeline(600)
			for i := range 10_000 {
				sut.Add(anchorSec, int64(1_000_000+i))
			}

			Expect(sut.pipeline.Window(anchorSec + 1).Count()).To(Equal(uint64(10_000)))
			Expect(sut.pipeline.Tree().NodeCount()).To(Equal(1))
		})

		It("leaves the current second out of the window", func() {
			sut.forPipeline(600)
			sut.Add(anchorSec, 5_000_000)

			Expect(sut.pipeline.Window(anchorSec).Count()).To(BeZero())
			Expect(sut.pipeline.Window(anchorSec + 1).Count()).To(Equal(uint64(1)))
		})

		It("keeps only the trailing window as it advances", func() {
			sut.forPipeline(600)
			sut.AddPerSecond(anchorSec, 7200, 1)

			Expect(sut.pipeline.Window(anchorSec + 7200).Count()).To(Equal(uint64(3600)))
		})
	})

	Context("corrections (spec 5.2)", func() {
		It("merges a late event into its second rather than replacing it", func() {
			sut.forPipeline(600)
			sut.AddPerSecond(anchorSec, 100, 1)
			sut.pipeline.Seal()

			sut.Add(anchorSec+10, 750_000_000)

			window := sut.pipeline.Window(anchorSec + 100)
			Expect(window.Count()).To(Equal(uint64(101)))
			Expect(window.Max()).To(Equal(uint64(750_000_000)))
		})

		It("corrects the oldest second still in the window", func() {
			sut.forPipeline(600)
			sut.AddPerSecond(anchorSec, 3600, 1)
			now := anchorSec + 3600

			sut.Add(now-ddhistogram.WindowSeconds, 800_000_000)

			Expect(sut.pipeline.Window(now).Max()).To(Equal(uint64(800_000_000)))
		})

		It("rejects a correction to a second that has already expired", func() {
			sut.forPipeline(600)
			sut.AddPerSecond(anchorSec, 6000, 1)
			now := anchorSec + 6000
			sut.pipeline.Advance(now)

			err := sut.pipeline.Add((sut.pipeline.Horizon()-1)*ddhistogram.NanosPerSecond, 5_000_000)
			Expect(err).To(MatchError(ddhistogram.ErrExpired))
		})

		It("still accepts a correction at the retention horizon", func() {
			sut.forPipeline(600)
			sut.AddPerSecond(anchorSec, 6000, 1)
			sut.pipeline.Advance(anchorSec + 6000)

			err := sut.pipeline.Add(sut.pipeline.Horizon()*ddhistogram.NanosPerSecond, 5_000_000)
			Expect(err).ToNot(HaveOccurred())
		})

		It("keeps retention at window length plus the maximum allowed lateness (spec 11.6)", func() {
			for _, maxLateness := range []uint64{0, 1, 600, 7200} {
				sut.forPipeline(maxLateness)
				now := anchorSec + 50_000
				sut.Add(now, 5_000_000)
				sut.pipeline.Advance(now)

				Expect(sut.pipeline.Horizon()).To(Equal(now-ddhistogram.WindowSeconds-maxLateness),
					fmt.Sprintf("with max lateness %d", maxLateness))
			}
		})

		It("applies a correction arriving exactly at the maximum allowed lateness", func() {
			const maxLateness = uint64(600)
			sut.forPipeline(maxLateness)
			sut.AddPerSecond(anchorSec, 6000, 1)
			now := anchorSec + 6000
			sut.pipeline.Advance(now)

			// the oldest second still inside the window, corrected as late as
			// the contract allows
			oldest := now - ddhistogram.WindowSeconds
			Expect(sut.pipeline.Add(oldest*ddhistogram.NanosPerSecond, 850_000_000)).To(Succeed())

			Expect(sut.pipeline.Window(now).Max()).To(Equal(uint64(850_000_000)))
		})
	})

	Context("differential test against the naive oracle (spec 8.4)", func() {
		It("agrees at every query step over a stream including late corrections", func() {
			events := generateEventStream(1, 60_000, 0.05)

			sut.forPipeline(600)
			oracle := newNaiveOracle()

			step := 0
			for _, e := range events {
				Expect(sut.pipeline.Add(e.timestampNS, e.latencyNS)).To(Succeed())
				oracle.add(e.timestampNS/ddhistogram.NanosPerSecond, e.latencyNS)

				step++
				if step%2000 != 0 {
					continue
				}

				now := e.timestampNS/ddhistogram.NanosPerSecond + 1
				sut.expectQuantilesWithinAlpha(oracle, now, fmt.Sprintf("at step %d", step))
			}
		})
	})

	Context("out of order equivalence (spec 8.5)", func() {
		It("produces identical results and an identical tree for a shuffled stream", func() {
			events := generateEventStream(2, 20_000, 0.1)

			ordered := feed(events)

			shuffled := make([]event, len(events))
			copy(shuffled, events)
			rand.New(rand.NewPCG(8, 9)).Shuffle(len(shuffled), func(i, j int) {
				shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
			})
			outOfOrder := feed(shuffled)

			now := lastSecond(events) + 1
			Expect(outOfOrder.Window(now).Equal(ordered.Window(now))).To(BeTrue())
			Expect(outOfOrder.Tree().Structure()).To(Equal(ordered.Tree().Structure()))
			Expect(outOfOrder.Tree().CheckInvariants()).To(Succeed())
		})
	})

	Context("distributed merge (spec 5.3 / 8.8)", func() {
		It("matches a single process run under several shard layouts", func() {
			events := generateEventStream(3, 20_000, 0.05)
			single := feed(events)
			now := lastSecond(events) + 1
			expected := single.Window(now)

			for _, shards := range []int{2, 3, 5, 8, 16} {
				merged := feedSharded(events, shards)

				Expect(merged.Window(now).Equal(expected)).To(BeTrue(),
					fmt.Sprintf("with %d shards", shards))
			}
		})

		It("pools distributions across series keys for a global rollup", func() {
			slow := feed(generateEventStreamAt(anchorSec, 5_000, 400_000_000))
			fast := feed(generateEventStreamAt(anchorSec, 5_000, 4_000_000))
			now := anchorSec + 100

			pooled := slow.Window(now).Clone()
			pooled.Merge(fast.Window(now))

			Expect(pooled.Count()).To(Equal(slow.Window(now).Count() + fast.Window(now).Count()))
			Expect(pooled.Min()).To(Equal(min(slow.Window(now).Min(), fast.Window(now).Min())))
			Expect(pooled.Max()).To(Equal(max(slow.Window(now).Max(), fast.Window(now).Max())))
		})
	})

	Context("retention (spec 4.6 / 8.7)", func() {
		It("does not change query results", func() {
			events := generateEventStream(4, 20_000, 0.0)
			now := lastSecond(events) + 1

			withoutEviction := feed(events)
			expected := withoutEviction.Window(now)

			withEviction := ddhistogram.NewPipeline(600)
			for _, e := range events {
				Expect(withEviction.Add(e.timestampNS, e.latencyNS)).To(Succeed())
				withEviction.Advance(e.timestampNS / ddhistogram.NanosPerSecond)
			}

			Expect(withEviction.Window(now).Equal(expected)).To(BeTrue())
		})

		It("keeps the node count flat over a long run", func() {
			sut.forPipeline(600)
			var peak int
			for offset := range uint64(30_000) {
				sec := anchorSec + offset
				sut.Add(sec, 5_000_000)
				sut.pipeline.Advance(sec)
				if offset > 10_000 {
					sut.pipeline.Seal()
					peak = max(peak, sut.pipeline.Tree().NodeCount())
				}
			}

			Expect(peak).To(BeNumerically("<", 5000))
		})
	})

	Context("edge cases (spec 8.6)", func() {
		It("is empty at epoch zero", func() {
			sut.forPipeline(600)
			Expect(sut.pipeline.Window(0).Count()).To(BeZero())
		})

		It("clamps the window start at epoch zero for an early now", func() {
			sut.forPipeline(600)
			sut.Add(5, 5_000_000)

			Expect(sut.pipeline.Window(10).Count()).To(Equal(uint64(1)))
		})

		It("does not evict before a full retention span has elapsed", func() {
			sut.forPipeline(600)
			sut.Add(anchorSec, 5_000_000)
			sut.pipeline.Advance(100)

			Expect(sut.pipeline.Horizon()).To(BeZero())
		})

		It("rejects a partial sketch for an already evicted second", func() {
			sut.forPipeline(600)
			sut.AddPerSecond(anchorSec, 6000, 1)
			sut.pipeline.Advance(anchorSec + 6000)

			err := sut.pipeline.AddPartial(sut.pipeline.Horizon()-1, ddhistogram.NewSketch())
			Expect(err).To(MatchError(ddhistogram.ErrExpired))
		})

		It("is undefined over an empty window", func() {
			sut.forPipeline(600)

			for _, q := range sut.pipeline.Quantiles(anchorSec, targetQuantiles()) {
				Expect(math.IsNaN(q)).To(BeTrue())
			}
		})

		It("answers a single value exactly at both extremes", func() {
			sut.forPipeline(600)
			sut.Add(anchorSec, 12_345_678)

			got := sut.pipeline.Quantiles(anchorSec+1, targetQuantiles())
			Expect(got[0]).To(Equal(float64(12_345_678)))
			Expect(got[len(got)-1]).To(Equal(float64(12_345_678)))
		})

		It("handles a window straddling a /52 block boundary", func() {
			boundary := (anchorSec >> 12 << 12) + 4096
			sut.forPipeline(600)
			sut.AddPerSecond(boundary-1800, 3600, 2)

			Expect(sut.pipeline.Window(boundary + 1800).Count()).To(Equal(uint64(7200)))
		})
	})
})

type pipelineSut struct {
	pipeline *ddhistogram.Pipeline
}

func (s *pipelineSut) forPipeline(maxLatenessSec uint64) {
	s.pipeline = ddhistogram.NewPipeline(maxLatenessSec)
}

func (s *pipelineSut) Add(sec uint64, latencyNS int64) {
	Expect(s.pipeline.Add(sec*ddhistogram.NanosPerSecond, latencyNS)).To(Succeed())
}

func (s *pipelineSut) AddPerSecond(from uint64, seconds int, perSecond int) {
	for i := range seconds {
		for j := range perSecond {
			s.Add(from+uint64(i), int64(1_000_000+i*1000+j))
		}
	}
}

func (s *pipelineSut) expectQuantilesWithinAlpha(oracle *naiveOracle, now uint64, context string) {
	from := uint64(0)
	if now > ddhistogram.WindowSeconds {
		from = now - ddhistogram.WindowSeconds
	}

	got := s.pipeline.Quantiles(now, targetQuantiles())
	exact := oracle.quantiles(from, now-1, targetQuantiles())

	for i, q := range targetQuantiles() {
		if math.IsNaN(exact[i]) {
			Expect(math.IsNaN(got[i])).To(BeTrue(), context)
			continue
		}
		if q <= 0.0 || q >= 1.0 {
			Expect(got[i]).To(Equal(exact[i]), fmt.Sprintf("%s: q %v must be exact", context, q))
			continue
		}
		Expect(math.Abs(got[i]-exact[i])).To(BeNumerically("<=", ddhistogram.Alpha*exact[i]),
			fmt.Sprintf("%s: at q %v got %v exact %v", context, q, got[i], exact[i]))
	}
}

// naiveOracle is the reference of spec 8.4: it keeps every raw value per second
// and recomputes the window by brute force. Slow by design.
type naiveOracle struct {
	seconds map[uint64][]int64
}

func newNaiveOracle() *naiveOracle {
	return &naiveOracle{seconds: make(map[uint64][]int64)}
}

func (o *naiveOracle) add(sec uint64, latencyNS int64) {
	o.seconds[sec] = append(o.seconds[sec], latencyNS)
}

func (o *naiveOracle) quantiles(from, to uint64, qs []float64) []float64 {
	var all []int64
	for sec, values := range o.seconds {
		if sec >= from && sec <= to {
			all = append(all, values...)
		}
	}

	out := make([]float64, len(qs))
	if len(all) == 0 {
		for i := range out {
			out[i] = math.NaN()
		}
		return out
	}

	sort.Slice(all, func(i, j int) bool { return all[i] < all[j] })
	for i, q := range qs {
		out[i] = float64(exactNearestRank(all, q))
	}
	return out
}

type event struct {
	timestampNS uint64
	latencyNS   int64
}

// generateEventStream produces a mostly ordered stream in which lateFraction of
// the events arrive after the second they belong to has been aggregated.
func generateEventStream(seed uint64, count int, lateFraction float64) []event {
	randomizer := rand.New(rand.NewPCG(seed, 4242))
	events := make([]event, 0, count)

	sec := anchorSec
	for i := range count {
		if i%6 == 0 {
			sec++
		}
		eventSec := sec
		if randomizer.Float64() < lateFraction {
			// belongs to an earlier second that has already been sealed
			eventSec = sec - (randomizer.Uint64N(2000) + 1)
		}
		latency := int64(math.Exp(math.Log(40_000_000) + 0.6*randomizer.NormFloat64()))
		events = append(events, event{
			timestampNS: eventSec*ddhistogram.NanosPerSecond + randomizer.Uint64N(ddhistogram.NanosPerSecond),
			latencyNS:   latency,
		})
	}
	return events
}

func generateEventStreamAt(from uint64, count int, medianNS int64) []event {
	randomizer := rand.New(rand.NewPCG(5, 4242))
	events := make([]event, 0, count)
	for i := range count {
		sec := from + uint64(i%60)
		latency := int64(math.Exp(math.Log(float64(medianNS)) + 0.4*randomizer.NormFloat64()))
		events = append(events, event{timestampNS: sec * ddhistogram.NanosPerSecond, latencyNS: latency})
	}
	return events
}

func feed(events []event) *ddhistogram.Pipeline {
	pipeline := ddhistogram.NewPipeline(4000)
	for _, e := range events {
		Expect(pipeline.Add(e.timestampNS, e.latencyNS)).To(Succeed())
	}
	return pipeline
}

// feedSharded models sharding by ingestion: several processes each build a
// partial sketch per second, and a merge stage folds the partials into the leaf.
func feedSharded(events []event, shards int) *ddhistogram.Pipeline {
	partials := make([]map[uint64]*ddhistogram.Sketch, shards)
	for i := range partials {
		partials[i] = make(map[uint64]*ddhistogram.Sketch)
	}

	randomizer := rand.New(rand.NewPCG(77, 99))
	for _, e := range events {
		shard := partials[randomizer.IntN(shards)]
		sec := e.timestampNS / ddhistogram.NanosPerSecond
		sketch, found := shard[sec]
		if !found {
			sketch = ddhistogram.NewSketch()
			shard[sec] = sketch
		}
		Expect(sketch.InsertLatency(e.latencyNS)).To(Succeed())
	}

	pipeline := ddhistogram.NewPipeline(4000)
	for _, shard := range partials {
		for sec, sketch := range shard {
			Expect(pipeline.AddPartial(sec, sketch)).To(Succeed())
		}
	}
	return pipeline
}

func lastSecond(events []event) uint64 {
	latest := uint64(0)
	for _, e := range events {
		latest = max(latest, e.timestampNS/ddhistogram.NanosPerSecond)
	}
	return latest
}
