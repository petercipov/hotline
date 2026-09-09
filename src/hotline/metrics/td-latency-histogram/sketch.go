// Package tdlatencyhistogram holds the t-digest algorithm and adapts it to the
// windowed quantile pipeline, so the same radix tree and the same window
// machinery can be driven with a t-digest instead of a DDSketch, and the two
// compared.
//
// TDigest is the algorithm; Sketch is the pipeline facing digest that wraps it
// and carries exact count, sum, min and max alongside.
package tdlatencyhistogram

import (
	"errors"
	"fmt"
	"math"
	"unsafe"

	"hotline/metrics/radixtree"
)

// ErrNegativeLatency rejects a negative measurement. Latencies are non
// negative; a negative input is a bug and is rejected loudly rather than
// silently folded in.
var ErrNegativeLatency = errors.New("tdlatencyhistogram: negative latency")

// Default t-digest tuning. Compression 100 is the delta the size comparison in
// the design notes was quoted against.
const (
	DefaultCompression = 100
	DefaultBufferSize  = 500
)

// Sketch wraps the TDigest in this package so it can be indexed by the radix
// tree in place of a DDSketch, and the two compared on precision and memory
// over identical event streams. It is the mirror of ddlatencyhistogram.Sketch.
//
// It is NOT a commutative monoid, and that is the whole point of having it.
// Merge re-clusters centroids, so it is lossy and order dependent: folding the
// same measurements in a different grouping or a different order yields a
// different digest. Under the tree that means the answer can depend on how a
// window happens to decompose into blocks and on the order partial results
// arrive, which is exactly the reproducibility a bucket histogram buys you.
// The digestcompare package quantifies the gap rather than assuming it.
//
// Exact count, sum, min and max are carried alongside, as they are for Sketch,
// so p0 and p100 stay exact for both and the comparison isolates the interior
// quantiles where the two actually differ.
type Sketch struct {
	digest *TDigest
	count  uint64
	sum    uint64
	min    uint64
	max    uint64
}

// NewSketch returns an empty t-digest backed sketch at the default tuning. It
// satisfies radixtree.Factory[*Sketch], so radixtree.New(NewSketch) works.
func NewSketch() *Sketch {
	return NewSketchSized(DefaultCompression, DefaultBufferSize)()
}

// NewTDigestSized returns a constructor at a chosen compression, for sweeping
// the accuracy against memory tradeoff.
func NewSketchSized(compression, bufferSize int) radixtree.Factory[*Sketch] {
	return func() *Sketch {
		return &Sketch{
			digest: NewTDigestWeightScaled(compression, bufferSize),
			min:    math.MaxUint64,
		}
	}
}

func (d *Sketch) Count() uint64 { return d.count }
func (d *Sketch) Sum() uint64   { return d.sum }

// Min is the exact minimum, or math.MaxUint64 when empty.
func (d *Sketch) Min() uint64 { return d.min }

// Max is the exact maximum, or 0 when empty.
func (d *Sketch) Max() uint64 { return d.max }

// Centroids exposes the compressed state, for inspection and equality.
func (d *Sketch) Centroids() []Centroid { return d.digest.ToCentroids() }

// Insert adds one measurement in nanoseconds.
func (d *Sketch) Insert(vNS uint64) {
	d.count++
	d.sum += vNS
	if vNS < d.min {
		d.min = vNS
	}
	if vNS > d.max {
		d.max = vNS
	}
	d.digest.AddToBuffer(float64(vNS), 1)
}

// InsertLatency adds one measurement, rejecting a negative value.
func (d *Sketch) InsertLatency(latencyNS int64) error {
	if latencyNS < 0 {
		return fmt.Errorf("%w: %d", ErrNegativeLatency, latencyNS)
	}
	d.Insert(uint64(latencyNS))
	return nil
}

// Merge folds other in by replaying its centroids as weighted entries, which is
// the only merge a t-digest has. The replay re-clusters, so information is lost
// here in a way that Sketch.Merge does not lose it: the scalars below stay
// exact, but the centroids do not survive a regrouping unchanged.
func (d *Sketch) Merge(other *Sketch) {
	for _, centroid := range other.digest.ToCentroids() {
		d.digest.AddToBuffer(centroid.Mean, centroid.Weight)
	}
	d.count += other.count
	d.sum += other.sum
	if other.min < d.min {
		d.min = other.min
	}
	if other.max > d.max {
		d.max = other.max
	}
}

// Clone returns a deep copy.
func (d *Sketch) Clone() *Sketch {
	clone := *d
	clone.digest = d.digest.Clone()
	return &clone
}

// Equal compares the scalars and the compressed centroids exactly.
func (d *Sketch) Equal(other *Sketch) bool {
	if d.count != other.count || d.sum != other.sum || d.min != other.min || d.max != other.max {
		return false
	}

	mine, theirs := d.digest.ToCentroids(), other.digest.ToCentroids()
	if len(mine) != len(theirs) {
		return false
	}
	for i := range mine {
		if mine[i] != theirs[i] {
			return false
		}
	}
	return true
}

// Quantiles answers every requested quantile, qs sorted ascending. p0 and p100
// come from the exact scalars, as they do for Sketch.
func (d *Sketch) Quantiles(qs []float64) []float64 {
	out := make([]float64, len(qs))
	if d.count == 0 {
		for i := range out {
			out[i] = math.NaN()
		}
		return out
	}

	for i, q := range qs {
		switch {
		case q <= 0.0:
			out[i] = float64(d.min)
		case q >= 1.0:
			out[i] = float64(d.max)
		default:
			out[i] = d.digest.Quantile(q)
		}
	}
	return out
}

// SizeInBytes is the retained footprint. It is bounded by compression rather
// than by the spread of the data, which is the tradeoff against Sketch, whose
// map grows with the number of distinct buckets the data touches.
func (d *Sketch) SizeInBytes() int {
	return int(unsafe.Sizeof(*d)) + d.digest.SizeInBytes()
}

// The tree indexes sketches, and asks nothing of them but that they merge.
var _ radixtree.Mergeable[*Sketch] = (*Sketch)(nil)
