package ddlatencyhistogram

import (
	"math"
	"testing"
)

func TestMergeSkipsZeroCountBuckets(t *testing.T) {
	// a zero count bucket can only arrive from a hand built sketch; it must
	// never reach the destination map
	donor := NewSketch()
	donor.buckets[400] = 0

	acc := NewSketch()
	acc.Insert(5_000_000)
	acc.Merge(donor)

	if _, found := acc.buckets[400]; found {
		t.Fatalf("merge propagated a zero count bucket")
	}
}

func TestEqualDistinguishesBucketsOfTheSameLength(t *testing.T) {
	// identical scalars, same number of buckets, different indexes
	left := NewSketch()
	left.count, left.sum, left.min, left.max = 1, 5_000_000, 5_000_000, 5_000_000
	left.buckets[400] = 1

	right := NewSketch()
	right.count, right.sum, right.min, right.max = 1, 5_000_000, 5_000_000, 5_000_000
	right.buckets[401] = 1

	if left.Equal(right) {
		t.Fatalf("sketches with different bucket indexes compared equal")
	}
}

func TestLnGammaMatchesTheStandardLibrary(t *testing.T) {
	// the pinned literal must be the value this platform's ln() produces, or
	// the pinned constant would silently shift every bucket boundary
	if computed := math.Log(Gamma); computed != LnGamma {
		t.Fatalf("LnGamma is %v, math.Log(Gamma) is %v", LnGamma, computed)
	}
}
