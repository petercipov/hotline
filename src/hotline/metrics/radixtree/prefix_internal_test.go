package radixtree

import (
	"fmt"
	"testing"
)

// Spec 9.5 — radix prefix arithmetic over epoch seconds.
const (
	t1 = uint64(1788776132) // 0x000000006a9e8ec4
	t2 = t1 + 1
	t3 = t1 + 3600
)

func TestLongestCommonPrefix(t *testing.T) {
	cases := []struct {
		name      string
		other     uint64
		key       uint64
		prefixLen int
	}{
		{"one second apart shares a /60 block", t2, 0x000000006a9e8ec0, 60},
		{"one hour apart shares a /48 block", t3, 0x000000006a9e0000, 48},
	}

	leaf := prefix{key: t1, prefixLen: keyBits}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := longestCommonPrefix(leaf, c.other)
			if got.key != c.key || got.prefixLen != c.prefixLen {
				t.Fatalf("got {key: %#016x, prefixLen: %d}, want {key: %#016x, prefixLen: %d}",
					got.key, got.prefixLen, c.key, c.prefixLen)
			}
		})
	}
}

func TestSlotOf(t *testing.T) {
	cases := []struct {
		prefixLen int
		slot      uint64
	}{
		{52, 14},
		{56, 12},
		{60, 4},
	}

	for _, c := range cases {
		t.Run(fmt.Sprintf("prefix_len %d", c.prefixLen), func(t *testing.T) {
			p := prefix{key: t1 & prefixMask(c.prefixLen), prefixLen: c.prefixLen}
			if got := slotOf(p, t1); got != c.slot {
				t.Fatalf("slotOf(prefix_len=%d, t1) = %d, want %d", c.prefixLen, got, c.slot)
			}
		})
	}
}

func TestPrefixLenIsAlwaysNibbleAligned(t *testing.T) {
	leaf := prefix{key: t1, prefixLen: keyBits}
	for delta := uint64(1); delta < 5000; delta++ {
		got := longestCommonPrefix(leaf, t1+delta)
		if got.prefixLen%radixBits != 0 {
			t.Fatalf("delta %d produced prefix_len %d, not a multiple of %d", delta, got.prefixLen, radixBits)
		}
		if got.key&^prefixMask(got.prefixLen) != 0 {
			t.Fatalf("delta %d produced key %#x with bits set below prefix_len %d", delta, got.key, got.prefixLen)
		}
		if !got.contains(t1) || !got.contains(t1+delta) {
			t.Fatalf("delta %d produced a prefix that does not contain both keys", delta)
		}
	}
}

// Spec 9.6 — range decomposition. A 3600 second window over a fully populated
// tree must resolve to 45 block aggregates for this alignment, and over all
// 4096 alignments to min 15, average 43.1, max 45.
func TestRangeDecomposition(t *testing.T) {
	tree := New(NewTally)
	from, to := t1-3599, t1
	for ts := from; ts <= to; ts++ {
		tree.Update(ts, TallyOf(ts))
	}

	_, blocks := tree.aggregateRange(from, to)
	if blocks != 45 {
		t.Fatalf("window [t1-3599, t1] decomposed into %d blocks, want 45", blocks)
	}
}

func TestRangeDecompositionOverAllAlignments(t *testing.T) {
	base := t1 &^ uint64(0xFFFF) // snap to a /48 boundary so all 4096 offsets fit
	tree := New(NewTally)
	for ts := base; ts < base+3600+4096; ts++ {
		tree.Update(ts, TallyOf(ts))
	}

	minBlocks, maxBlocks, total := 1<<30, 0, 0
	for offset := range uint64(4096) {
		from := base + offset
		_, blocks := tree.aggregateRange(from, from+3599)
		total += blocks
		minBlocks = min(minBlocks, blocks)
		maxBlocks = max(maxBlocks, blocks)
	}

	average := float64(total) / 4096
	if minBlocks != 15 || maxBlocks != 45 {
		t.Fatalf("blocks ranged over [%d, %d], want [15, 45]", minBlocks, maxBlocks)
	}
	if average < 43.05 || average > 43.15 {
		t.Fatalf("average blocks %.2f, want 43.1", average)
	}
}
