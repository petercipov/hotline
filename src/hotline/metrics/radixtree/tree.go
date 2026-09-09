// Package radixtree indexes one mergeable aggregate per second over a radix 16
// tree of unix epoch second keys, so that a range aggregate over an arbitrary
// window costs O(log n) merges instead of O(n).
//
// It is deliberately ignorant of what it is aggregating: the only operation it
// performs on the payload is Merge. Anything that forms a commutative monoid
// can be indexed by it, which is what lets a latency sketch and a t-digest be
// swapped underneath the same window machinery and compared.
package radixtree

import (
	"errors"
	"fmt"
	"math/bits"
	"strings"
	"unsafe"
)

// ErrInvariant reports a structural invariant violation in the tree.
var ErrInvariant = errors.New("radixtree: invariant violated")

const (
	// radix is the number of children per node: use it where you need a count.
	radix = 16
	// radixBits is the number of key bits consumed per level: use it where you
	// need a bit position.
	radixBits = 4
	// keyBits is the width of the timestamp key, in unix epoch seconds.
	keyBits = 64
)

// prefix identifies the range of timestamps sharing a bit prefix. All bits
// below prefixLen are zero, and prefixLen is always a multiple of radixBits.
// prefixLen 0 is the root, covering all time; prefixLen keyBits is a leaf,
// covering one exact second.
type prefix struct {
	key       uint64
	prefixLen int
}

func wildcardBits(prefixLen int) int { return keyBits - prefixLen }

func prefixMask(prefixLen int) uint64 {
	if prefixLen == 0 {
		return 0
	}
	wildcard := wildcardBits(prefixLen)
	return (^uint64(0) >> wildcard) << wildcard
}

func (p prefix) contains(ts uint64) bool {
	mask := prefixMask(p.prefixLen)
	return (p.key & mask) == (ts & mask)
}

// span is the number of seconds the prefix covers: 1 for a leaf, 16 for a /60,
// 256 for a /56 and so on.
func (p prefix) span() uint64 { return uint64(1) << wildcardBits(p.prefixLen) }

func (p prefix) first() uint64 { return p.key & prefixMask(p.prefixLen) }

func (p prefix) last() uint64 { return p.first() + p.span() - 1 }

func slotOf(p prefix, ts uint64) uint64 {
	return (ts >> wildcardBits(p.prefixLen+radixBits)) & (radix - 1)
}

// longestCommonPrefix snaps the shared prefix of p and ts down to a nibble
// boundary. That snapping is why prefixLen is always nibble aligned, and why a
// 51 bit common prefix produces a /48 node.
func longestCommonPrefix(p prefix, ts uint64) prefix {
	shared := min(bits.LeadingZeros64(ts^p.key), p.prefixLen)
	length := shared - (shared % radixBits)
	return prefix{key: ts & prefixMask(length), prefixLen: length}
}

// childPtr identifies a child and carries the aggregate over everything under
// it. The aggregate lives here in the parent, not in the child node. A leaf is
// just a childPtr with prefixLen == keyBits and no node; only internal nodes
// are stored records.
type childPtr[T Mergeable[T]] struct {
	pfx  prefix
	agg  T
	node *node[T]
}

func (c *childPtr[T]) isLeaf() bool { return c.node == nil }

type node[T Mergeable[T]] struct {
	pfx      prefix
	children [radix]*childPtr[T]
}

// fold recomputes this node's aggregate from its current children. Slot order
// is fixed, so the fold is reproducible for any payload type, even one whose
// merge is order dependent.
func (n *node[T]) fold(empty Factory[T]) T {
	acc := empty()
	for _, child := range n.children {
		if child != nil {
			acc.Merge(child.agg)
		}
	}
	return acc
}

// Tree indexes one aggregate per second so that a range aggregate over an
// arbitrary window costs O(log n) merges instead of O(n).
//
// Path compression is mandatory: a node exists only where the data actually
// branches, so a node's child may be many levels deeper than the node itself.
// With epoch second keys the top eight nibbles are never materialized at all.
//
// It requires only Mergeable: nothing here knows what a bucket or a centroid
// is.
type Tree[T Mergeable[T]] struct {
	root  *node[T]
	empty Factory[T]
}

// New builds a tree whose aggregates are produced by empty. T is inferred from
// the constructor, so New(NewSketch) is enough.
func New[T Mergeable[T]](empty Factory[T]) *Tree[T] {
	return &Tree[T]{
		root:  &node[T]{pfx: prefix{key: 0, prefixLen: 0}},
		empty: empty,
	}
}

// Update applies a new or corrected aggregate for one second. It touches only the
// nodes on the root to leaf path, and every node aggregate on that path is
// recomputed from its current children rather than patched incrementally —
// which is what makes a non invertible operator work.
func (t *Tree[T]) Update(tsSec uint64, agg T) {
	slot := slotOf(t.root.pfx, tsSec)
	// the root is never collapsed, even with one child
	t.root.children[slot] = t.insertInto(t.root.children[slot], tsSec, agg)
}

func (t *Tree[T]) insertInto(target *childPtr[T], tsSec uint64, agg T) *childPtr[T] {
	if target == nil {
		return newLeaf(tsSec, agg)
	}

	if !target.pfx.contains(tsSec) {
		return t.split(target, tsSec, agg)
	}

	if target.isLeaf() {
		// contains plus leaf means the key is exactly tsSec
		target.agg = agg
		return target
	}

	slot := slotOf(target.pfx, tsSec)
	target.node.children[slot] = t.insertInto(target.node.children[slot], tsSec, agg)
	target.agg = target.node.fold(t.empty)
	return collapse(target)
}

func newLeaf[T Mergeable[T]](tsSec uint64, agg T) *childPtr[T] {
	return &childPtr[T]{pfx: prefix{key: tsSec, prefixLen: keyBits}, agg: agg}
}

// split creates a new node at the longest common prefix of the existing subtree
// and the new leaf, and points at both.
func (t *Tree[T]) split(existing *childPtr[T], tsSec uint64, agg T) *childPtr[T] {
	branch := longestCommonPrefix(existing.pfx, tsSec)

	created := &node[T]{pfx: branch}
	created.children[slotOf(branch, existing.pfx.key)] = existing
	created.children[slotOf(branch, tsSec)] = newLeaf(tsSec, agg)

	return &childPtr[T]{pfx: branch, agg: created.fold(t.empty), node: created}
}

// collapse enforces the path compression invariant that every non root node has
// at least two occupied children.
func collapse[T Mergeable[T]](target *childPtr[T]) *childPtr[T] {
	occupied := 0
	var only *childPtr[T]
	for _, child := range target.node.children {
		if child != nil {
			occupied++
			only = child
		}
	}

	switch occupied {
	case 0:
		return nil
	case 1:
		return only // reparent the single child into the grandparent
	default:
		return target
	}
}

// AggregateRange folds every second in the inclusive range [fromSec, toSec] into
// one aggregate. A child whose prefix is entirely inside the range contributes one
// precomputed merge with no descent; only the two boundaries walk down.
func (t *Tree[T]) AggregateRange(fromSec, toSec uint64) T {
	result, _ := t.aggregateRange(fromSec, toSec)
	return result
}

// aggregateRange also reports how many block aggregates the range decomposed
// into, which is what the decomposition tests assert on.
func (t *Tree[T]) aggregateRange(fromSec, toSec uint64) (T, int) {
	result := t.empty()
	blocks := 0
	if fromSec > toSec {
		return result, blocks
	}

	var visit func(target *childPtr[T])
	visit = func(target *childPtr[T]) {
		first, last := target.pfx.first(), target.pfx.last()
		if last < fromSec || first > toSec {
			return
		}
		if fromSec <= first && last <= toSec {
			result.Merge(target.agg)
			blocks++
			return
		}
		// partial overlap; a leaf spans one second so it is never partial
		for _, child := range target.node.children {
			if child != nil {
				visit(child)
			}
		}
	}

	for _, child := range t.root.children {
		if child != nil {
			visit(child)
		}
	}
	return result, blocks
}

// Structure renders the tree shape without any aggregate content. The shape is
// canonical — determined solely by the set of keys — so two trees built from
// the same seconds in any order must render identically.
func (t *Tree[T]) Structure() string {
	var render func(target *childPtr[T], depth int, out *strings.Builder)
	render = func(target *childPtr[T], depth int, out *strings.Builder) {
		fmt.Fprintf(out, "%s%#016x/%d\n", strings.Repeat(" ", depth), target.pfx.key, target.pfx.prefixLen)
		if target.isLeaf() {
			return
		}
		for _, child := range target.node.children {
			if child != nil {
				render(child, depth+1, out)
			}
		}
	}

	var out strings.Builder
	for _, child := range t.root.children {
		if child != nil {
			render(child, 0, &out)
		}
	}
	return out.String()
}

// walk visits every stored childPtr, leaves included.
func (t *Tree[T]) walk(visit func(target *childPtr[T])) {
	var descend func(target *childPtr[T])
	descend = func(target *childPtr[T]) {
		visit(target)
		if target.isLeaf() {
			return
		}
		for _, child := range target.node.children {
			if child != nil {
				descend(child)
			}
		}
	}
	for _, child := range t.root.children {
		if child != nil {
			descend(child)
		}
	}
}

// NodeCount counts stored records: internal nodes plus leaves.
func (t *Tree[T]) NodeCount() int {
	count := 0
	t.walk(func(*childPtr[T]) { count++ })
	return count
}

// SizeInBytes is the retained footprint of the whole index: every childPtr and
// internal node, plus the aggregate each one carries. Aggregates dominate,
// which is the point — it makes the memory cost of a payload choice measurable.
func (t *Tree[T]) SizeInBytes() int {
	total := int(unsafe.Sizeof(node[T]{})) // the root
	t.walk(func(target *childPtr[T]) {
		total += int(unsafe.Sizeof(childPtr[T]{})) + target.agg.SizeInBytes()
		if !target.isLeaf() {
			total += int(unsafe.Sizeof(node[T]{}))
		}
	})
	return total
}

// CheckInvariants verifies the structural invariants of the tree. Callers
// should assert on it after every mutation in debug builds.
func (t *Tree[T]) CheckInvariants() error {
	var check func(target *childPtr[T], parent prefix, slot uint64) error
	check = func(target *childPtr[T], parent prefix, slot uint64) error {
		p := target.pfx
		switch {
		case p.prefixLen%radixBits != 0:
			return fmt.Errorf("%w: prefix_len %d is not a multiple of %d", ErrInvariant, p.prefixLen, radixBits)
		case p.prefixLen <= parent.prefixLen:
			return fmt.Errorf("%w: child prefix_len %d is not deeper than parent %d", ErrInvariant, p.prefixLen, parent.prefixLen)
		case p.key&^prefixMask(p.prefixLen) != 0:
			return fmt.Errorf("%w: key %#x has bits set below prefix_len %d", ErrInvariant, p.key, p.prefixLen)
		case !parent.contains(p.key):
			return fmt.Errorf("%w: parent %#x/%d does not contain child key %#x", ErrInvariant, parent.key, parent.prefixLen, p.key)
		case slotOf(parent, p.key) != slot:
			return fmt.Errorf("%w: child %#x sits in slot %d, belongs in %d", ErrInvariant, p.key, slot, slotOf(parent, p.key))
		}

		if target.isLeaf() {
			return nil
		}

		occupied := 0
		for childSlot := range uint64(radix) {
			child := target.node.children[childSlot]
			if child == nil {
				continue
			}
			occupied++
			if err := check(child, p, childSlot); err != nil {
				return err
			}
		}
		if occupied < 2 {
			return fmt.Errorf("%w: non root node %#x/%d has %d occupied children, want at least 2", ErrInvariant,
				p.key, p.prefixLen, occupied)
		}
		if !target.agg.Equal(target.node.fold(t.empty)) {
			return fmt.Errorf("%w: aggregate of %#x/%d does not equal the fold of its children", ErrInvariant, p.key, p.prefixLen)
		}
		return nil
	}

	for slot := range uint64(radix) {
		if child := t.root.children[slot]; child != nil {
			if err := check(child, t.root.pfx, slot); err != nil {
				return err
			}
		}
	}
	return nil
}

// EvictBefore drops every second earlier than horizon and reclaims the nodes
// they leave empty. The tree does not need trimming for correctness — a range
// query simply never visits older nodes — so this is purely memory reclamation.
func (t *Tree[T]) EvictBefore(horizon uint64) {
	for slot, child := range t.root.children {
		if child != nil {
			t.root.children[slot] = t.evict(child, horizon)
		}
	}
}

func (t *Tree[T]) evict(target *childPtr[T], horizon uint64) *childPtr[T] {
	if target.pfx.last() < horizon {
		return nil // an aligned expired span is a subtree drop, not a scan
	}
	if target.pfx.first() >= horizon || target.isLeaf() {
		return target
	}

	for slot, child := range target.node.children {
		if child != nil {
			target.node.children[slot] = t.evict(child, horizon)
		}
	}
	target.agg = target.node.fold(t.empty)
	return collapse(target)
}
