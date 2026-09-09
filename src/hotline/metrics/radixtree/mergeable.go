package radixtree

// Mergeable is everything the tree requires of the value it indexes: a way to
// fold two of them together, and a way to compare the result.
//
// The tree deliberately asks for nothing else. It is a cache of pre-folded
// partial results arranged by time, so the only operation it ever performs on
// the payload is Merge. Anything that is a commutative monoid under Merge can
// be indexed by it.
//
// The recursive type parameter (T Mergeable[T]) keeps the payload type concrete
// all the way down, so a tree of one payload type cannot accidentally merge a
// value of another — the failure mode a plain interface would turn into a
// runtime type assertion.
type Mergeable[T any] interface {
	// Merge folds other into the receiver.
	Merge(other T)
	// Equal reports exact equality. The tree asserts on this when checking
	// that a node aggregate still equals the fold of its children.
	Equal(other T) bool
	// SizeInBytes is the retained heap footprint, so the cost of a payload
	// choice can be measured rather than estimated.
	SizeInBytes() int
}

// Factory constructs an empty aggregate. It is the identity element supplier:
// the tree calls it whenever it needs a fresh accumulator to fold into.
type Factory[T Mergeable[T]] func() T
