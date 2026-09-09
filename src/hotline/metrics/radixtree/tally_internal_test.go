package radixtree

import "unsafe"

// Tally is the smallest possible Mergeable, used as the payload throughout this
// package's tests. Keeping the tree's tests on a two field counter rather than
// on a real sketch is the point: it proves the tree needs nothing but a
// commutative monoid, and it keeps a tree bug from hiding behind sketch
// behaviour (or the reverse).
//
// It is exported so the external spec package can use it too. Test files are
// invisible to importers, so this does not widen the package's API.
type Tally struct {
	count uint64
	sum   uint64
}

// NewTally is a Factory[*Tally]: the empty tally is the identity of Merge.
func NewTally() *Tally { return &Tally{} }

// TallyOf builds a tally over the given values.
func TallyOf(values ...uint64) *Tally {
	t := NewTally()
	for _, v := range values {
		t.Add(v)
	}
	return t
}

func (t *Tally) Add(value uint64) {
	t.count++
	t.sum += value
}

func (t *Tally) Count() uint64 { return t.count }
func (t *Tally) Sum() uint64   { return t.sum }

func (t *Tally) Merge(other *Tally) {
	t.count += other.count
	t.sum += other.sum
}

func (t *Tally) Equal(other *Tally) bool { return *t == *other }

func (t *Tally) SizeInBytes() int { return int(unsafe.Sizeof(*t)) }

// The double is held to the same contract as any real payload.
var _ Mergeable[*Tally] = (*Tally)(nil)
