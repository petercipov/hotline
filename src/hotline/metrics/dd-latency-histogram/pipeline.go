package ddlatencyhistogram

import (
	"errors"
	"fmt"

	"hotline/metrics/radixtree"
)

const (
	// NanosPerSecond converts an event timestamp to a tree key.
	NanosPerSecond = 1_000_000_000
	// WindowSeconds is the length of the sliding window.
	WindowSeconds = 3600
)

// ErrExpired rejects a correction landing on a second that has already been
// evicted, so that a dropped correction is an explicit error rather than an
// accident.
var ErrExpired = errors.New("ddlatencyhistogram: second already evicted")

// The tree indexes sketches, and asks nothing of them but that they merge.
var _ radixtree.Mergeable[*Sketch] = (*Sketch)(nil)

// Pipeline turns a stream of latency measurements into windowed quantiles for
// one series key.
//
// Events are bucketed by second into an open sketch, which is where the volume
// collapses: any number of events per second becomes one sketch, so the tree
// never sees the event rate, only one leaf per second. Sealing writes that
// sketch to the tree leaf; a late event re-runs the same two steps for its
// second, and the tree's unwind recomputes only that leaf's ancestors.
type Pipeline struct {
	tree *radixtree.Tree[*Sketch]
	// open keeps per second sketches addressable for as long as corrections
	// are accepted, so a correction merges into its second rather than
	// replacing it with just the late event
	open        map[uint64]*Sketch
	dirty       map[uint64]struct{}
	maxLateness uint64
	horizon     uint64
}

// NewPipeline creates a pipeline retaining maxLatenessSec seconds beyond the
// window, which is how late a correction may arrive and still be applied.
func NewPipeline(maxLatenessSec uint64) *Pipeline {
	return &Pipeline{
		tree:        radixtree.New(NewSketch),
		open:        make(map[uint64]*Sketch),
		dirty:       make(map[uint64]struct{}),
		maxLateness: maxLatenessSec,
	}
}

// Tree exposes the time index, for invariant checks and structural comparison.
func (p *Pipeline) Tree() *radixtree.Tree[*Sketch] { return p.tree }

// Add records one measurement. It is safe to call for a second that has already
// been sealed: the measurement merges into that second and the leaf is resealed.
func (p *Pipeline) Add(timestampNS uint64, latencyNS int64) error {
	tsSec := timestampNS / NanosPerSecond
	if tsSec < p.horizon {
		return fmt.Errorf("%w: second %d is below horizon %d", ErrExpired, tsSec, p.horizon)
	}
	if err := p.secondSketch(tsSec).InsertLatency(latencyNS); err != nil {
		return err
	}

	p.dirty[tsSec] = struct{}{}
	return nil
}

// AddPartial folds a partial sketch produced elsewhere into one second. This is
// the merge stage for sharding by ingestion: order of arrival is irrelevant
// because the operation is commutative.
func (p *Pipeline) AddPartial(tsSec uint64, partial *Sketch) error {
	if tsSec < p.horizon {
		return fmt.Errorf("%w: second %d is below horizon %d", ErrExpired, tsSec, p.horizon)
	}

	p.secondSketch(tsSec).Merge(partial)
	p.dirty[tsSec] = struct{}{}
	return nil
}

func (p *Pipeline) secondSketch(tsSec uint64) *Sketch {
	sketch, found := p.open[tsSec]
	if !found {
		sketch = NewSketch()
		p.open[tsSec] = sketch
	}
	return sketch
}

// Seal writes every second touched since the last seal into the tree.
func (p *Pipeline) Seal() {
	for tsSec := range p.dirty {
		p.tree.Update(tsSec, p.open[tsSec].Clone())
		delete(p.dirty, tsSec)
	}
}

// Window folds the sliding window ending at nowSec into one sketch. The window
// covers [nowSec-WindowSeconds, nowSec-1]: the current second is still open.
func (p *Pipeline) Window(nowSec uint64) *Sketch {
	p.Seal()
	if nowSec == 0 {
		return NewSketch()
	}

	from := uint64(0)
	if nowSec > WindowSeconds {
		from = nowSec - WindowSeconds
	}
	return p.tree.AggregateRange(from, nowSec-1)
}

// Quantiles answers every requested quantile over the window ending at nowSec.
// qs MUST be sorted ascending.
func (p *Pipeline) Quantiles(nowSec uint64, qs []float64) []float64 {
	return p.Window(nowSec).Quantiles(qs)
}

// Advance moves the retention horizon to match nowSec and reclaims everything
// below it. Retention is window length plus the maximum allowed lateness, so a
// correction that is still acceptable can never land on an evicted second.
func (p *Pipeline) Advance(nowSec uint64) {
	retained := uint64(WindowSeconds) + p.maxLateness
	if nowSec <= retained {
		return
	}

	p.Seal()
	p.horizon = nowSec - retained
	p.tree.EvictBefore(p.horizon)
	for tsSec := range p.open {
		if tsSec < p.horizon {
			delete(p.open, tsSec)
		}
	}
}

// Horizon is the oldest second still accepting corrections.
func (p *Pipeline) Horizon() uint64 { return p.horizon }
