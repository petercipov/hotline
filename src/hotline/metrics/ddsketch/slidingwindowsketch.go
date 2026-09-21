package ddsketch

import (
	"errors"
	"fmt"
	"time"

	"hotline/metrics/radixtree"
)

const (
	// NanosPerSecond converts an event timestamp to a tree key.
	NanosPerSecond = 1_000_000_000
	// DefaultWindow is one hour: long enough for a latency SLO to be read off
	// a single window, short enough to react within a shift.
	DefaultWindow = time.Hour
)

// ErrExpired rejects a correction landing on a second that has already been
// evicted, so that a dropped correction is an explicit error rather than an
// accident.
var ErrExpired = errors.New("ddsketch: second already evicted")

// ErrInvalidWindow rejects a window that cannot be counted in whole seconds.
var ErrInvalidWindow = errors.New("ddsketch: invalid window")

// ErrInvalidLateness rejects a lateness that cannot be counted in whole
// seconds.
var ErrInvalidLateness = errors.New("ddsketch: invalid lateness")

// SlidingWindowSketch turns a stream of latency measurements into quantiles
// over a sliding window, for one series key.
//
// Events are bucketed by second into an open sketch, which is where the volume
// collapses: any number of events per second becomes one sketch, so the tree
// never sees the event rate, only one leaf per second. Sealing writes that
// sketch to the tree leaf; a late event re-runs the same two steps for its
// second, and the tree's unwind recomputes only that leaf's ancestors.
type SlidingWindowSketch struct {
	rng Range
	// the window is held in both units it is used in: the seconds the time
	// index is keyed by, and the length it was configured with. Both are set
	// once at construction and never diverge.
	windowSec uint64
	windowLen time.Duration
	tree      *radixtree.Tree[*Sketch]
	// open keeps per second sketches addressable for as long as corrections
	// are accepted, so a correction merges into its second rather than
	// replacing it with just the late event
	open        map[uint64]*Sketch
	dirty       map[uint64]struct{}
	maxLateness uint64
	horizon     uint64
}

// NewSlidingWindowSketch creates a sliding window sketch over DefaultRange reporting over
// a window of the given length, retaining maxLateness beyond it, which is how
// late a measurement may arrive and still be applied. A zero lateness keeps a second
// writable only for as long as it is inside the window.
//
// Retention is window plus lateness, and costs one leaf per retained second
// that carries traffic, so the two together are what the memory of a series is
// proportional to. DefaultWindow is the usual window.
func NewSlidingWindowSketch(window, maxLateness time.Duration) (*SlidingWindowSketch, error) {
	return NewSlidingWindowSketchInRange(window, maxLateness, DefaultRange())
}

// NewSlidingWindowSketchInRange creates a sliding window sketch resolving latencies over r.
// Every sketch it builds, and every accumulator the tree folds into, carries
// that range, so nothing inside one of them can mix spans.
func NewSlidingWindowSketchInRange(window, maxLateness time.Duration, r Range) (*SlidingWindowSketch, error) {
	windowSec, err := windowSeconds(window)
	if err != nil {
		return nil, err
	}
	latenessSec, err := latenessSeconds(maxLateness)
	if err != nil {
		return nil, err
	}
	if err := r.Validate(); err != nil {
		return nil, err
	}

	return &SlidingWindowSketch{
		rng:         r,
		windowSec:   windowSec,
		windowLen:   window,
		tree:        radixtree.New(func() *Sketch { return NewSketchInRange(r) }),
		open:        make(map[uint64]*Sketch),
		dirty:       make(map[uint64]struct{}),
		maxLateness: latenessSec,
	}, nil
}

// windowSeconds converts a configured window to the seconds the tree is keyed
// by. A window below one second cannot be expressed in leaves at all, and one
// that is not a whole number of seconds would have to be truncated, which would
// report over a window the caller did not ask for. Both are refused rather than
// rounded.
func windowSeconds(window time.Duration) (uint64, error) {
	switch {
	case window < time.Second:
		return 0, fmt.Errorf("%w: window must be at least 1s, got %s", ErrInvalidWindow, window)
	case window%time.Second != 0:
		return 0, fmt.Errorf("%w: window must be a whole number of seconds, got %s",
			ErrInvalidWindow, window)
	}
	return uint64(window / time.Second), nil
}

// latenessSeconds converts a configured lateness to seconds of retention behind
// the window. Zero is the meaningful floor rather than one second: it accepts
// no correction once a second has left the window. A sub second lateness would
// truncate to that same zero, so it is refused instead of silently becoming a
// contract the caller did not ask for.
func latenessSeconds(lateness time.Duration) (uint64, error) {
	switch {
	case lateness < 0:
		return 0, fmt.Errorf("%w: lateness must not be negative, got %s",
			ErrInvalidLateness, lateness)
	case lateness%time.Second != 0:
		return 0, fmt.Errorf("%w: lateness must be a whole number of seconds, got %s",
			ErrInvalidLateness, lateness)
	}
	return uint64(lateness / time.Second), nil
}

// WindowLength is the sliding window this one reports over.
func (w *SlidingWindowSketch) WindowLength() time.Duration { return w.windowLen }

// Tree exposes the time index, for invariant checks and structural comparison.
func (w *SlidingWindowSketch) Tree() *radixtree.Tree[*Sketch] { return w.tree }

// Add records one measurement. It is safe to call for a second that has already
// been sealed: the measurement merges into that second and the leaf is resealed.
func (w *SlidingWindowSketch) Add(timestampNS uint64, latencyNS int64) error {
	tsSec := timestampNS / NanosPerSecond
	if tsSec < w.horizon {
		return fmt.Errorf("%w: second %d is below horizon %d", ErrExpired, tsSec, w.horizon)
	}
	if err := w.secondSketch(tsSec).InsertLatency(latencyNS); err != nil {
		return err
	}

	w.dirty[tsSec] = struct{}{}
	return nil
}

// AddPartial folds a partial sketch produced elsewhere into one second. This is
// the merge stage for sharding by ingestion: order of arrival is irrelevant
// because the operation is commutative.
func (w *SlidingWindowSketch) AddPartial(tsSec uint64, partial *Sketch) error {
	if tsSec < w.horizon {
		return fmt.Errorf("%w: second %d is below horizon %d", ErrExpired, tsSec, w.horizon)
	}
	// a bucket index means the same under any range, but zeros and the clamp
	// do not, so folding a foreign span in would be silent nonsense
	if partial.rng != w.rng {
		return fmt.Errorf("%w: partial resolves [%d, %d], window resolves [%d, %d]",
			ErrRangeMismatch, partial.rng.MinNS, partial.rng.MaxNS, w.rng.MinNS, w.rng.MaxNS)
	}

	w.secondSketch(tsSec).Merge(partial)
	w.dirty[tsSec] = struct{}{}
	return nil
}

func (w *SlidingWindowSketch) secondSketch(tsSec uint64) *Sketch {
	sketch, found := w.open[tsSec]
	if !found {
		sketch = NewSketchInRange(w.rng)
		w.open[tsSec] = sketch
	}
	return sketch
}

// Seal writes every second touched since the last seal into the tree.
func (w *SlidingWindowSketch) Seal() {
	for tsSec := range w.dirty {
		w.tree.Update(tsSec, w.open[tsSec].Clone())
		delete(w.dirty, tsSec)
	}
}

// Window folds the sliding window ending at nowSec into one sketch. The window
// covers [nowSec-WindowLength(), nowSec-1]: the current second is still open.
func (w *SlidingWindowSketch) Window(nowSec uint64) *Sketch {
	w.Seal()
	if nowSec == 0 {
		return NewSketchInRange(w.rng)
	}

	from := uint64(0)
	if nowSec > w.windowSec {
		from = nowSec - w.windowSec
	}
	return w.tree.AggregateRange(from, nowSec-1)
}

// Quantiles answers every requested quantile over the window ending at nowSec.
// qs MUST be sorted ascending.
func (w *SlidingWindowSketch) Quantiles(nowSec uint64, qs []float64) []float64 {
	return w.Window(nowSec).Quantiles(qs)
}

// Advance moves the retention horizon to match nowSec and reclaims everything
// below it. Retention is window length plus the maximum allowed lateness, so a
// correction that is still acceptable can never land on an evicted second.
func (w *SlidingWindowSketch) Advance(nowSec uint64) {
	retained := w.windowSec + w.maxLateness
	if nowSec <= retained {
		return
	}

	w.Seal()
	w.horizon = nowSec - retained
	w.tree.EvictBefore(w.horizon)
	for tsSec := range w.open {
		if tsSec < w.horizon {
			delete(w.open, tsSec)
		}
	}
}

// Horizon is the oldest second still accepting corrections.
func (w *SlidingWindowSketch) Horizon() uint64 { return w.horizon }
