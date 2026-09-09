// Package latencyhistogram computes arbitrary percentiles over a sliding window
// of latency measurements.
//
// The unit of aggregation is a DDSketch: a log-bucket histogram whose merge is
// per-bucket integer addition, and therefore exactly associative and
// commutative. That is what lets partial results from any number of processes,
// per-second leaves, radix tree internal nodes and cross-shard rollups all use
// one fold, so moving a shard boundary cannot change the answer.
package ddlatencyhistogram

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"unsafe"
)

const (
	// Alpha is the relative error guaranteed on the reconstructed value.
	Alpha = 0.02
	// Gamma is the ratio between consecutive bucket bounds.
	Gamma = (1 + Alpha) / (1 - Alpha)
	// MinNS is the smallest value that gets its own bucket; below it values
	// are counted as zeros.
	MinNS = uint64(1_000)
	// MaxNS is the largest representable value; above it values are clamped
	// into the top bucket and counted as overflow.
	MaxNS = uint64(100_000_000_000)

	// LnGamma is the bucket width in log space. It is pinned as a literal
	// rather than computed, because ln() differs in the last ulp across
	// platforms and standard libraries, and since index() applies ceil, a one
	// ulp difference can flip the index for a value sitting exactly on a
	// bucket boundary.
	LnGamma = 0.040005334613699206
)

// ErrNegativeLatency rejects a negative measurement.
var ErrNegativeLatency = errors.New("ddlatencyhistogram: negative latency")

// Index returns the bucket covering vNS. Bucket i covers (Gamma^(i-1), Gamma^i].
// The caller is responsible for keeping vNS inside [MinNS, MaxNS]; Insert does
// the clamping.
func Index(vNS uint64) int32 {
	return int32(math.Ceil(math.Log(float64(vNS)) / LnGamma))
}

// BucketValue returns the representative of bucket i: the log space midpoint,
// which is what makes the guarantee hold in both directions, so that for any v
// in bucket i, (1-Alpha)*v <= BucketValue(i) <= (1+Alpha)*v.
func BucketValue(i int32) float64 {
	return 2 * math.Pow(Gamma, float64(i)) / (Gamma + 1)
}

// Sketch is a DDSketch with exact min/max/sum carried alongside. Every field is
// independently a commutative monoid under integer +, min or max, so the struct
// is one.
type Sketch struct {
	buckets map[int32]uint64
	zeros   uint64
	count   uint64
	sum     uint64
	min     uint64
	max     uint64

	// clamping counters, so a misconfigured range is detectable in production
	underflowCount uint64
	overflowCount  uint64
}

// NewSketch returns the empty sketch, which is the identity element of Merge.
func NewSketch() *Sketch {
	return &Sketch{
		buckets: make(map[int32]uint64),
		min:     math.MaxUint64,
	}
}

func (s *Sketch) Count() uint64     { return s.count }
func (s *Sketch) Sum() uint64       { return s.sum }
func (s *Sketch) Zeros() uint64     { return s.zeros }
func (s *Sketch) Underflow() uint64 { return s.underflowCount }
func (s *Sketch) Overflow() uint64  { return s.overflowCount }

// Min is the exact minimum, or math.MaxUint64 when the sketch is empty.
func (s *Sketch) Min() uint64 { return s.min }

// Max is the exact maximum, or 0 when the sketch is empty.
func (s *Sketch) Max() uint64 { return s.max }

// Buckets returns a copy of the populated bucket index to count map.
func (s *Sketch) Buckets() map[int32]uint64 {
	buckets := make(map[int32]uint64, len(s.buckets))
	for i, c := range s.buckets {
		buckets[i] = c
	}
	return buckets
}

// Insert adds one measurement in nanoseconds.
func (s *Sketch) Insert(vNS uint64) {
	s.count++
	s.sum += vNS
	if vNS < s.min {
		s.min = vNS
	}
	if vNS > s.max {
		s.max = vNS
	}

	if vNS < MinNS {
		s.zeros++
		s.underflowCount++
		return
	}

	clamped := vNS
	if clamped > MaxNS {
		clamped = MaxNS
		s.overflowCount++
	}
	s.buckets[Index(clamped)]++
}

// InsertLatency adds one measurement, rejecting a negative value. Latencies are
// non negative; a negative input is a bug and is rejected loudly rather than
// silently bucketed.
func (s *Sketch) InsertLatency(latencyNS int64) error {
	if latencyNS < 0 {
		return fmt.Errorf("%w: %d", ErrNegativeLatency, latencyNS)
	}
	s.Insert(uint64(latencyNS))
	return nil
}

// Merge folds other into s. This is the operation the whole design rests on:
// it is commutative, associative, has the empty sketch as a two sided identity
// and loses nothing at any merge depth.
func (s *Sketch) Merge(other *Sketch) {
	for i, c := range other.buckets {
		if c == 0 {
			continue
		}
		s.buckets[i] += c
	}
	s.zeros += other.zeros
	s.count += other.count
	s.sum += other.sum
	if other.min < s.min {
		s.min = other.min
	}
	if other.max > s.max {
		s.max = other.max
	}
	s.underflowCount += other.underflowCount
	s.overflowCount += other.overflowCount
}

// Clone returns a deep copy, so callers can merge into it without disturbing
// the original.
func (s *Sketch) Clone() *Sketch {
	clone := *s
	clone.buckets = s.Buckets()
	return &clone
}

// Equal compares every field bit exactly. Merge determinism is asserted on this,
// never on extracted quantiles.
func (s *Sketch) Equal(other *Sketch) bool {
	if s.zeros != other.zeros ||
		s.count != other.count ||
		s.sum != other.sum ||
		s.min != other.min ||
		s.max != other.max ||
		s.underflowCount != other.underflowCount ||
		s.overflowCount != other.overflowCount ||
		len(s.buckets) != len(other.buckets) {
		return false
	}
	for i, c := range s.buckets {
		if other.buckets[i] != c {
			return false
		}
	}
	return true
}

// SizeInBytes is the retained footprint: the fixed scalars plus the sparse
// bucket map. mapOverheadFactor covers Go's map load factor and per bucket
// tophash bookkeeping — the map is what actually scales, and it is why buckets
// MUST stay sparse: at leaf level only ~50-350 of the 462 possible indexes are
// populated, and a dense array would triple this number.
func (s *Sketch) SizeInBytes() int {
	const mapOverheadFactor = 1.4
	var index int32
	var count uint64
	entry := int(unsafe.Sizeof(index) + unsafe.Sizeof(count))

	return int(unsafe.Sizeof(*s)) + int(float64(len(s.buckets)*entry)*mapOverheadFactor)
}

// SortedBucketIndexes returns the populated indexes ascending, which is also
// ascending by value.
func (s *Sketch) SortedBucketIndexes() []int32 {
	indexes := make([]int32, 0, len(s.buckets))
	for i := range s.buckets {
		indexes = append(indexes, i)
	}
	sort.Slice(indexes, func(a, b int) bool { return indexes[a] < indexes[b] })
	return indexes
}

// Quantiles extracts every requested quantile in one pass. qs MUST be sorted
// ascending. The convention is nearest rank: the q-quantile is the smallest
// value v such that at least ceil(q*count) values are <= v.
//
// Asking for p20, p50, p70, p99, p99.9 and p100 costs the same as asking for
// p99 alone. p0 and p100 are exact, taken from min and max rather than from a
// bucket. An empty sketch yields NaN for every quantile.
func (s *Sketch) Quantiles(qs []float64) []float64 {
	out := make([]float64, len(qs))
	if s.count == 0 {
		for i := range out {
			out[i] = math.NaN()
		}
		return out
	}

	targets := make([]uint64, len(qs))
	for i, q := range qs {
		target := uint64(math.Ceil(q * float64(s.count)))
		if q <= 0 || target < 1 {
			target = 1
		}
		if target > s.count {
			target = s.count
		}
		targets[i] = target
	}

	// the zeros bucket sorts before every real bucket
	cumulative := s.zeros
	next := 0
	for next < len(qs) && targets[next] <= cumulative {
		out[next] = 0
		next++
	}

	for _, i := range s.SortedBucketIndexes() {
		cumulative += s.buckets[i]
		for next < len(qs) && targets[next] <= cumulative {
			out[next] = BucketValue(i)
			next++
		}
		if next == len(qs) {
			break
		}
	}

	// exact extremes override the approximation
	for i, q := range qs {
		if q <= 0.0 {
			out[i] = float64(s.min)
		}
		if q >= 1.0 {
			out[i] = float64(s.max)
		}
	}
	return out
}
