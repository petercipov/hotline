package radixtree

import (
	"errors"
	"testing"
)

// The tree invariants only fire on a corrupted tree, which the public API
// cannot produce, so these cases assemble the corruption by hand.

func leafAt(ts uint64) *childPtr[*Tally] {
	return newLeaf(ts, TallyOf(ts))
}

// twoLeafNode builds a well formed node holding two adjacent leaves, hung off
// the root in its correct slot.
func twoLeafNode() (*Tree[*Tally], *childPtr[*Tally]) {
	tree := New(NewTally)
	tree.Update(t1, TallyOf(t1))
	tree.Update(t1+1, TallyOf(t1+1))
	return tree, tree.root.children[slotOf(tree.root.pfx, t1)]
}

func expectInvariantError(t *testing.T, tree *Tree[*Tally], why string) {
	t.Helper()
	err := tree.CheckInvariants()
	if err == nil {
		t.Fatalf("expected an invariant violation for %s, got none", why)
	}
	if !errors.Is(err, ErrInvariant) {
		t.Fatalf("expected %s to report ErrInvariant, got %v", why, err)
	}
}

func TestCheckInvariantsAcceptsAWellFormedTree(t *testing.T) {
	tree, _ := twoLeafNode()
	if err := tree.CheckInvariants(); err != nil {
		t.Fatalf("well formed tree reported %v", err)
	}
}

func TestCheckInvariantsRejectsPrefixLenNotNibbleAligned(t *testing.T) {
	tree, branch := twoLeafNode()
	branch.pfx.prefixLen = 59
	expectInvariantError(t, tree, "a prefix_len that is not a multiple of radixBits")
}

func TestCheckInvariantsRejectsAChildNoDeeperThanItsParent(t *testing.T) {
	tree, branch := twoLeafNode()
	branch.node.children[slotOf(branch.pfx, t1)].pfx = branch.pfx
	expectInvariantError(t, tree, "a child no deeper than its parent")
}

func TestCheckInvariantsRejectsKeyBitsBelowPrefixLen(t *testing.T) {
	tree, branch := twoLeafNode()
	branch.pfx.key |= 1 // a /60 key must have its bottom nibble clear
	expectInvariantError(t, tree, "a key with bits set below prefix_len")
}

func TestCheckInvariantsRejectsAChildOutsideItsParent(t *testing.T) {
	tree, branch := twoLeafNode()
	slot := slotOf(branch.pfx, t1)
	branch.node.children[slot] = leafAt(t1 + 1_000_000)
	expectInvariantError(t, tree, "a child outside its parent's prefix")
}

func TestCheckInvariantsRejectsAChildInTheWrongSlot(t *testing.T) {
	tree, branch := twoLeafNode()
	from := slotOf(branch.pfx, t1)
	to := (from + 1) % radix
	for branch.node.children[to] != nil {
		to = (to + 1) % radix
	}
	branch.node.children[to] = branch.node.children[from]
	branch.node.children[from] = nil
	expectInvariantError(t, tree, "a child sitting in the wrong slot")
}

func TestCheckInvariantsRejectsAnUncompressedNode(t *testing.T) {
	tree, branch := twoLeafNode()
	branch.node.children[slotOf(branch.pfx, t1)] = nil
	branch.agg = branch.node.fold(NewTally)
	expectInvariantError(t, tree, "a non root node with fewer than two children")
}

func TestCheckInvariantsRejectsAStaleAggregate(t *testing.T) {
	tree, branch := twoLeafNode()
	branch.agg = NewTally()
	expectInvariantError(t, tree, "an aggregate that is not the fold of its children")
}

func TestCheckInvariantsDescendsIntoNestedNodes(t *testing.T) {
	tree := New(NewTally)
	for _, ts := range []uint64{t1, t1 + 1, t1 + 300, t1 + 90_000} {
		tree.Update(ts, TallyOf(ts))
	}
	if err := tree.CheckInvariants(); err != nil {
		t.Fatalf("well formed nested tree reported %v", err)
	}

	deepest := tree.root.children[slotOf(tree.root.pfx, t1)]
	for !deepest.isLeaf() {
		next := deepest
		for _, child := range deepest.node.children {
			if child != nil && !child.isLeaf() {
				next = child
			}
		}
		if next == deepest {
			break
		}
		deepest = next
	}
	deepest.agg = NewTally()
	expectInvariantError(t, tree, "a stale aggregate on a nested node")
}
