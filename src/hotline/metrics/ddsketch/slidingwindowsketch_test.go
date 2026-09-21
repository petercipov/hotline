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

// anchorSec is the epoch second the normative prefix vectors are anchored on.
const anchorSec = uint64(1788776132)

// defaultWindowSec is the default window as a count of seconds, which is what
// window arithmetic over tree keys is done in.
const defaultWindowSec = uint64(ddsketch.DefaultWindow / time.Second)

// secondsIn counts the whole seconds of a window or a lateness, which is the
// unit the specs address the time index in. A duration below one second counts
// as none, which is what a zero lateness means.
func secondsIn(d time.Duration) uint64 {
	if d < time.Second {
		return 0
	}
	return uint64(d / time.Second)
}

func targetQuantiles() []float64 {
	return []float64{0.0, 0.2, 0.5, 0.7, 0.99, 0.999, 1.0}
}

var _ = Describe("SlidingWindowSketch", func() {
	sut := slidingWindowSketchSut{}

	Context("ingestion", func() {
		It("rejects a negative latency loudly", func() {
			sut.forDefaultWindow(10 * time.Minute)

			err := sut.sliding.Add(anchorSec*ddsketch.NanosPerSecond, -1)
			Expect(err).To(MatchError(ddsketch.ErrNegativeLatency))
		})

		It("collapses any number of events in a second into one leaf", func() {
			sut.forDefaultWindow(10 * time.Minute)
			for i := range 10_000 {
				sut.Add(anchorSec, int64(1_000_000+i))
			}

			Expect(sut.sliding.Window(anchorSec + 1).Count()).To(Equal(uint64(10_000)))
			Expect(sut.sliding.Tree().NodeCount()).To(Equal(1))
		})

		It("leaves the current second out of the window", func() {
			sut.forDefaultWindow(10 * time.Minute)
			sut.Add(anchorSec, 5_000_000)

			Expect(sut.sliding.Window(anchorSec).Count()).To(BeZero())
			Expect(sut.sliding.Window(anchorSec + 1).Count()).To(Equal(uint64(1)))
		})

		It("keeps only the trailing window as it advances", func() {
			sut.forDefaultWindow(10 * time.Minute)
			sut.AddPerSecond(anchorSec, 7200, 1)

			Expect(sut.sliding.Window(anchorSec + 7200).Count()).To(Equal(uint64(3600)))
		})
	})

	Context("corrections (spec 5.2)", func() {
		It("merges a late event into its second rather than replacing it", func() {
			sut.forDefaultWindow(10 * time.Minute)
			sut.AddPerSecond(anchorSec, 100, 1)
			sut.sliding.Seal()

			sut.Add(anchorSec+10, 750_000_000)

			window := sut.sliding.Window(anchorSec + 100)
			Expect(window.Count()).To(Equal(uint64(101)))
			Expect(window.Max()).To(Equal(uint64(750_000_000)))
		})

		It("corrects the oldest second still in the window", func() {
			sut.forDefaultWindow(10 * time.Minute)
			sut.AddPerSecond(anchorSec, 3600, 1)
			now := anchorSec + 3600

			sut.Add(now-defaultWindowSec, 800_000_000)

			Expect(sut.sliding.Window(now).Max()).To(Equal(uint64(800_000_000)))
		})

		It("rejects a correction to a second that has already expired", func() {
			sut.forDefaultWindow(10 * time.Minute)
			sut.AddPerSecond(anchorSec, 6000, 1)
			now := anchorSec + 6000
			sut.sliding.Advance(now)

			err := sut.sliding.Add((sut.sliding.Horizon()-1)*ddsketch.NanosPerSecond, 5_000_000)
			Expect(err).To(MatchError(ddsketch.ErrExpired))
		})

		It("still accepts a correction at the retention horizon", func() {
			sut.forDefaultWindow(10 * time.Minute)
			sut.AddPerSecond(anchorSec, 6000, 1)
			sut.sliding.Advance(anchorSec + 6000)

			err := sut.sliding.Add(sut.sliding.Horizon()*ddsketch.NanosPerSecond, 5_000_000)
			Expect(err).ToNot(HaveOccurred())
		})

		It("keeps retention at window length plus the maximum allowed lateness (spec 11.6)", func() {
			for _, window := range []time.Duration{time.Second, time.Minute, ddsketch.DefaultWindow} {
				for _, maxLateness := range []time.Duration{0, time.Second, 10 * time.Minute, 2 * time.Hour} {
					sut.forWindow(window, maxLateness)
					now := anchorSec + 50_000
					sut.Add(now, 5_000_000)
					sut.sliding.Advance(now)

					windowSec := secondsIn(window)
					Expect(sut.sliding.Horizon()).To(Equal(now-windowSec-secondsIn(maxLateness)),
						fmt.Sprintf("with window %s and max lateness %s", window, maxLateness))
				}
			}
		})

		It("applies a correction arriving exactly at the maximum allowed lateness", func() {
			const maxLateness = 10 * time.Minute
			sut.forDefaultWindow(maxLateness)
			sut.AddPerSecond(anchorSec, 6000, 1)
			now := anchorSec + 6000
			sut.sliding.Advance(now)

			// the oldest second still inside the window, corrected as late as
			// the contract allows
			oldest := now - defaultWindowSec
			Expect(sut.sliding.Add(oldest*ddsketch.NanosPerSecond, 850_000_000)).To(Succeed())

			Expect(sut.sliding.Window(now).Max()).To(Equal(uint64(850_000_000)))
		})
	})

	Context("differential test against the naive oracle (spec 8.4)", func() {
		It("agrees at every query step over a stream including late corrections", func() {
			events := generateEventStream(1, 60_000, 0.05)

			sut.forDefaultWindow(10 * time.Minute)
			oracle := newNaiveOracle()

			step := 0
			for _, e := range events {
				Expect(sut.sliding.Add(e.timestampNS, e.latencyNS)).To(Succeed())
				oracle.add(e.timestampNS/ddsketch.NanosPerSecond, e.latencyNS)

				step++
				if step%2000 != 0 {
					continue
				}

				now := e.timestampNS/ddsketch.NanosPerSecond + 1
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

			withEviction, err := ddsketch.NewSlidingWindowSketch(ddsketch.DefaultWindow, 10*time.Minute)
			Expect(err).ToNot(HaveOccurred())
			for _, e := range events {
				Expect(withEviction.Add(e.timestampNS, e.latencyNS)).To(Succeed())
				withEviction.Advance(e.timestampNS / ddsketch.NanosPerSecond)
			}

			Expect(withEviction.Window(now).Equal(expected)).To(BeTrue())
		})

		It("keeps the node count flat over a long run", func() {
			sut.forDefaultWindow(10 * time.Minute)
			var peak int
			for offset := range uint64(30_000) {
				sec := anchorSec + offset
				sut.Add(sec, 5_000_000)
				sut.sliding.Advance(sec)
				if offset > 10_000 {
					sut.sliding.Seal()
					peak = max(peak, sut.sliding.Tree().NodeCount())
				}
			}

			Expect(peak).To(BeNumerically("<", 5000))
		})
	})

	Context("edge cases (spec 8.6)", func() {
		It("is empty at epoch zero", func() {
			sut.forDefaultWindow(10 * time.Minute)
			Expect(sut.sliding.Window(0).Count()).To(BeZero())
		})

		It("clamps the window start at epoch zero for an early now", func() {
			sut.forDefaultWindow(10 * time.Minute)
			sut.Add(5, 5_000_000)

			Expect(sut.sliding.Window(10).Count()).To(Equal(uint64(1)))
		})

		It("does not evict before a full retention span has elapsed", func() {
			sut.forDefaultWindow(10 * time.Minute)
			sut.Add(anchorSec, 5_000_000)
			sut.sliding.Advance(100)

			Expect(sut.sliding.Horizon()).To(BeZero())
		})

		It("rejects a partial sketch for an already evicted second", func() {
			sut.forDefaultWindow(10 * time.Minute)
			sut.AddPerSecond(anchorSec, 6000, 1)
			sut.sliding.Advance(anchorSec + 6000)

			err := sut.sliding.AddPartial(sut.sliding.Horizon()-1, ddsketch.NewSketch())
			Expect(err).To(MatchError(ddsketch.ErrExpired))
		})

		It("is undefined over an empty window", func() {
			sut.forDefaultWindow(10 * time.Minute)

			for _, q := range sut.sliding.Quantiles(anchorSec, targetQuantiles()) {
				Expect(math.IsNaN(q)).To(BeTrue())
			}
		})

		It("answers a single value exactly at both extremes", func() {
			sut.forDefaultWindow(10 * time.Minute)
			sut.Add(anchorSec, 12_345_678)

			got := sut.sliding.Quantiles(anchorSec+1, targetQuantiles())
			Expect(got[0]).To(Equal(float64(12_345_678)))
			Expect(got[len(got)-1]).To(Equal(float64(12_345_678)))
		})

		It("leaves a burst behind a long gap out of the window", func() {
			const gap = uint64(3 * 3600)
			sut.forDefaultWindow(10 * time.Minute)
			// ten fast latencies inside one second, then three silent hours,
			// then five slow ones inside one second
			for range 10 {
				sut.Add(anchorSec, 100_000_000)
			}
			for range 5 {
				sut.Add(anchorSec+gap, 900_000_000)
			}

			now := anchorSec + gap + 1
			window := sut.sliding.Window(now)
			Expect(window.Count()).To(Equal(uint64(5)))
			// the older burst is absent rather than merely outweighed: were it
			// folded in, the minimum would be the fast latency
			Expect(window.Min()).To(Equal(uint64(900_000_000)))

			p99 := sut.sliding.Quantiles(now, []float64{0.99})[0]
			Expect(math.Abs(p99 - 900_000_000)).To(BeNumerically("<=", ddsketch.Alpha*900_000_000))

			// reclaiming the stale second is invisible to the answer
			retained := sut.sliding.Tree().NodeCount()
			sut.sliding.Advance(now)
			Expect(sut.sliding.Tree().NodeCount()).To(BeNumerically("<", retained))
			Expect(sut.sliding.Window(now).Equal(window)).To(BeTrue())
		})

		It("is undefined once a gap has aged every measurement out", func() {
			sut.forDefaultWindow(10 * time.Minute)
			for range 10 {
				sut.Add(anchorSec, 100_000_000)
			}

			// a whole window later the series has gone quiet, which reads as no
			// answer rather than as the last one it had
			for _, q := range sut.sliding.Quantiles(anchorSec+defaultWindowSec+1, targetQuantiles()) {
				Expect(math.IsNaN(q)).To(BeTrue())
			}
		})

		It("handles a window straddling a /52 block boundary", func() {
			boundary := (anchorSec >> 12 << 12) + 4096
			sut.forDefaultWindow(10 * time.Minute)
			sut.AddPerSecond(boundary-1800, 3600, 2)

			Expect(sut.sliding.Window(boundary + 1800).Count()).To(Equal(uint64(7200)))
		})
	})

	Context("a configured window", func() {
		It("defaults to one hour", func() {
			sut.forDefaultWindow(10 * time.Minute)

			Expect(ddsketch.DefaultWindow).To(Equal(time.Hour))
			Expect(sut.sliding.WindowLength()).To(Equal(time.Hour))
		})

		It("reports the window it was given", func() {
			sut.forWindow(90*time.Second, 10*time.Minute)

			Expect(sut.sliding.WindowLength()).To(Equal(90 * time.Second))
		})

		It("keeps only the configured trailing window", func() {
			sut.forWindow(time.Minute, 10*time.Minute)
			sut.AddPerSecond(anchorSec, 7200, 1)

			Expect(sut.sliding.Window(anchorSec + 7200).Count()).To(Equal(uint64(60)))
		})

		It("reports over a single second window", func() {
			sut.forWindow(time.Second, 10*time.Minute)
			sut.AddPerSecond(anchorSec, 10, 1)

			// only the second that just closed
			window := sut.sliding.Window(anchorSec + 10)
			Expect(window.Count()).To(Equal(uint64(1)))
		})

		It("spans a window longer than a day", func() {
			const twoDays = uint64(48 * 3600)
			sut.forWindow(48*time.Hour, 10*time.Minute)
			sut.AddPerSecond(anchorSec, 100, 1)

			Expect(sut.sliding.Window(anchorSec + 100).Count()).To(Equal(uint64(100)))
			Expect(sut.sliding.Window(anchorSec + twoDays).Count()).To(Equal(uint64(100)))
			// one second past the window, the oldest second has dropped out
			Expect(sut.sliding.Window(anchorSec + twoDays + 1).Count()).To(Equal(uint64(99)))
		})

		It("still accepts a correction at the horizon of a short window", func() {
			sut.forWindow(time.Minute, 10*time.Minute)
			sut.AddPerSecond(anchorSec, 6000, 1)
			now := anchorSec + 6000
			sut.sliding.Advance(now)

			oldest := now - 60
			Expect(sut.sliding.Add(oldest*ddsketch.NanosPerSecond, 850_000_000)).To(Succeed())
			Expect(sut.sliding.Window(now).Max()).To(Equal(uint64(850_000_000)))
		})

		It("evicts everything a short window has left behind", func() {
			sut.forWindow(time.Minute, 0)
			sut.AddPerSecond(anchorSec, 600, 1)
			now := anchorSec + 600
			sut.sliding.Advance(now)

			Expect(sut.sliding.Horizon()).To(Equal(now - 60))
			err := sut.sliding.Add((now-61)*ddsketch.NanosPerSecond, 5_000_000)
			Expect(err).To(MatchError(ddsketch.ErrExpired))
		})

		It("refuses a window shorter than the second it is keyed by", func() {
			for _, window := range []time.Duration{0, -time.Hour, 500 * time.Millisecond} {
				_, err := ddsketch.NewSlidingWindowSketch(window, 10*time.Minute)
				Expect(err).To(MatchError(ddsketch.ErrInvalidWindow),
					fmt.Sprintf("with window %s", window))
			}
		})

		It("refuses a window that is not a whole number of seconds", func() {
			// truncating to 1s would report over a window the caller did not ask
			// for, so it is refused rather than rounded
			_, err := ddsketch.NewSlidingWindowSketch(1500*time.Millisecond, 10*time.Minute)
			Expect(err).To(MatchError(ddsketch.ErrInvalidWindow))
		})

		It("accepts a lateness of zero", func() {
			sut.forWindow(time.Minute, 0)

			now := anchorSec + 6000
			sut.Add(now, 5_000_000)
			sut.sliding.Advance(now)

			// nothing is writable behind the window at all
			Expect(sut.sliding.Horizon()).To(Equal(now - 60))
		})

		It("refuses a negative lateness", func() {
			_, err := ddsketch.NewSlidingWindowSketch(ddsketch.DefaultWindow, -time.Second)
			Expect(err).To(MatchError(ddsketch.ErrInvalidLateness))
		})

		It("refuses a lateness that is not a whole number of seconds", func() {
			_, err := ddsketch.NewSlidingWindowSketch(ddsketch.DefaultWindow, 1500*time.Millisecond)
			Expect(err).To(MatchError(ddsketch.ErrInvalidLateness))
		})

		It("refuses an invalid lateness over a configured range", func() {
			_, err := ddsketch.NewSlidingWindowSketchInRange(ddsketch.DefaultWindow, -time.Second, ddsketch.DefaultRange())
			Expect(err).To(MatchError(ddsketch.ErrInvalidLateness))
		})

		It("refuses an invalid window over a configured range", func() {
			_, err := ddsketch.NewSlidingWindowSketchInRange(0, 10*time.Minute, ddsketch.DefaultRange())
			Expect(err).To(MatchError(ddsketch.ErrInvalidWindow))
		})
	})
})

type slidingWindowSketchSut struct {
	sliding *ddsketch.SlidingWindowSketch
}

func (s *slidingWindowSketchSut) forDefaultWindow(maxLateness time.Duration) {
	s.forWindow(ddsketch.DefaultWindow, maxLateness)
}

func (s *slidingWindowSketchSut) forWindow(window, maxLateness time.Duration) {
	sliding, err := ddsketch.NewSlidingWindowSketch(window, maxLateness)
	Expect(err).ToNot(HaveOccurred())
	s.sliding = sliding
}

func (s *slidingWindowSketchSut) Add(sec uint64, latencyNS int64) {
	Expect(s.sliding.Add(sec*ddsketch.NanosPerSecond, latencyNS)).To(Succeed())
}

func (s *slidingWindowSketchSut) AddPerSecond(from uint64, seconds int, perSecond int) {
	for i := range seconds {
		for j := range perSecond {
			s.Add(from+uint64(i), int64(1_000_000+i*1000+j))
		}
	}
}

func (s *slidingWindowSketchSut) expectQuantilesWithinAlpha(oracle *naiveOracle, now uint64, context string) {
	windowSec := secondsIn(s.sliding.WindowLength())
	from := uint64(0)
	if now > windowSec {
		from = now - windowSec
	}

	got := s.sliding.Quantiles(now, targetQuantiles())
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
		Expect(math.Abs(got[i]-exact[i])).To(BeNumerically("<=", ddsketch.Alpha*exact[i]),
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
			timestampNS: eventSec*ddsketch.NanosPerSecond + randomizer.Uint64N(ddsketch.NanosPerSecond),
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
		events = append(events, event{timestampNS: sec * ddsketch.NanosPerSecond, latencyNS: latency})
	}
	return events
}

func feed(events []event) *ddsketch.SlidingWindowSketch {
	sliding, err := ddsketch.NewSlidingWindowSketch(ddsketch.DefaultWindow, 4000*time.Second)
	Expect(err).ToNot(HaveOccurred())
	for _, e := range events {
		Expect(sliding.Add(e.timestampNS, e.latencyNS)).To(Succeed())
	}
	return sliding
}

// feedSharded models sharding by ingestion: several processes each build a
// partial sketch per second, and a merge stage folds the partials into the leaf.
func feedSharded(events []event, shards int) *ddsketch.SlidingWindowSketch {
	partials := make([]map[uint64]*ddsketch.Sketch, shards)
	for i := range partials {
		partials[i] = make(map[uint64]*ddsketch.Sketch)
	}

	randomizer := rand.New(rand.NewPCG(77, 99))
	for _, e := range events {
		shard := partials[randomizer.IntN(shards)]
		sec := e.timestampNS / ddsketch.NanosPerSecond
		sketch, found := shard[sec]
		if !found {
			sketch = ddsketch.NewSketch()
			shard[sec] = sketch
		}
		Expect(sketch.InsertLatency(e.latencyNS)).To(Succeed())
	}

	sliding, err := ddsketch.NewSlidingWindowSketch(ddsketch.DefaultWindow, 4000*time.Second)
	Expect(err).ToNot(HaveOccurred())
	for _, shard := range partials {
		for sec, sketch := range shard {
			Expect(sliding.AddPartial(sec, sketch)).To(Succeed())
		}
	}
	return sliding
}

func lastSecond(events []event) uint64 {
	latest := uint64(0)
	for _, e := range events {
		latest = max(latest, e.timestampNS/ddsketch.NanosPerSecond)
	}
	return latest
}
