package radixtree_test

import (
	"fmt"
	"math/rand/v2"

	"hotline/metrics/radixtree"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const anchorSec = uint64(1788776132)

var _ = Describe("Tree", func() {
	sut := treeSut{}

	Context("update", func() {
		It("aggregates nothing for an empty tree", func() {
			sut.forEmptyTree()

			Expect(sut.AggregateRange(0, ^uint64(0)).Count()).To(BeZero())
			Expect(sut.tree.NodeCount()).To(BeZero())
		})

		It("stores a single second as one leaf hanging off the root", func() {
			sut.forEmptyTree()
			sut.Put(anchorSec, 5_000_000)

			Expect(sut.tree.NodeCount()).To(Equal(1))
			Expect(sut.AggregateRange(anchorSec, anchorSec).Count()).To(Equal(uint64(1)))
		})

		It("branches only where the data branches", func() {
			sut.forEmptyTree()
			sut.Put(anchorSec, 5_000_000)
			sut.Put(anchorSec+1, 6_000_000)

			// one /60 node plus two leaves; the levels between the root and
			// the /60 are never materialized
			Expect(sut.tree.NodeCount()).To(Equal(3))
		})

		It("replaces a leaf rather than accumulating into it", func() {
			sut.forEmptyTree()
			sut.Put(anchorSec, 5_000_000)
			sut.Put(anchorSec, 6_000_000)

			Expect(sut.AggregateRange(anchorSec, anchorSec).Count()).To(Equal(uint64(1)))
		})

		It("recomputes every ancestor aggregate from its children", func() {
			sut.forEmptyTree()
			sut.PutRange(anchorSec, 200)

			Expect(sut.tree.CheckInvariants()).To(Succeed())
			Expect(sut.AggregateRange(anchorSec, anchorSec+199).Count()).To(Equal(uint64(200)))
		})

		It("holds the invariants after every single mutation", func() {
			sut.forEmptyTree()
			randomizer := rand.New(rand.NewPCG(5, 11))
			for i := range 500 {
				sut.Put(anchorSec+uint64(randomizer.IntN(4000)), uint64(1_000_000+i))
				Expect(sut.tree.CheckInvariants()).To(Succeed(), fmt.Sprintf("after mutation %d", i))
			}
		})

		It("produces a canonical structure independent of insertion order", func() {
			seconds := make([]uint64, 0, 1000)
			for i := range 1000 {
				seconds = append(seconds, anchorSec+uint64(i)*7)
			}

			sut.forEmptyTree()
			for _, sec := range seconds {
				sut.Put(sec, 3_000_000)
			}
			ordered := sut.tree.Structure()

			randomizer := rand.New(rand.NewPCG(2, 3))
			randomizer.Shuffle(len(seconds), func(i, j int) {
				seconds[i], seconds[j] = seconds[j], seconds[i]
			})

			sut.forEmptyTree()
			for _, sec := range seconds {
				sut.Put(sec, 3_000_000)
			}

			Expect(sut.tree.Structure()).To(Equal(ordered))
		})
	})

	Context("aggregate range", func() {
		It("counts every inserted event over the full key range (spec 7)", func() {
			sut.forEmptyTree()
			sut.PutRange(anchorSec, 3600)

			Expect(sut.AggregateRange(0, ^uint64(0)).Count()).To(Equal(uint64(3600)))
		})

		It("is inclusive on both bounds", func() {
			sut.forEmptyTree()
			sut.PutRange(anchorSec, 10)

			Expect(sut.AggregateRange(anchorSec, anchorSec+9).Count()).To(Equal(uint64(10)))
			Expect(sut.AggregateRange(anchorSec+1, anchorSec+8).Count()).To(Equal(uint64(8)))
		})

		It("excludes data outside the window rather than treating it as wrong", func() {
			sut.forEmptyTree()
			sut.PutRange(anchorSec, 7200)

			Expect(sut.AggregateRange(anchorSec+3600, anchorSec+7199).Count()).To(Equal(uint64(3600)))
		})

		It("returns the empty aggregate for an inverted range", func() {
			sut.forEmptyTree()
			sut.PutRange(anchorSec, 10)

			Expect(sut.AggregateRange(anchorSec+5, anchorSec+1).Count()).To(BeZero())
		})

		It("matches a naive fold over the same seconds for random windows", func() {
			sut.forEmptyTree()
			sut.PutRange(anchorSec, 5000)

			randomizer := rand.New(rand.NewPCG(31, 41))
			for range 200 {
				from := anchorSec + uint64(randomizer.IntN(5000))
				to := from + uint64(randomizer.IntN(3600))

				Expect(sut.AggregateRange(from, to).Equal(sut.naiveFold(from, to))).To(BeTrue(),
					fmt.Sprintf("window [%d, %d]", from, to))
			}
		})

		It("survives a window straddling a /52 block boundary", func() {
			boundary := (anchorSec >> 12 << 12) + 4096 // a /52 block is 4096 seconds
			sut.forEmptyTree()
			sut.PutRange(boundary-1800, 3600)

			Expect(sut.AggregateRange(boundary-1800, boundary+1799).Count()).To(Equal(uint64(3600)))
		})
	})

	Context("corrections", func() {
		It("reflects a correction to a second already inside the window", func() {
			sut.forEmptyTree()
			sut.PutRange(anchorSec, 3600)

			sut.tree.Update(anchorSec, radixtree.TallyOf(1_000_000, 2_000_000))

			Expect(sut.AggregateRange(anchorSec, anchorSec+3599).Count()).To(Equal(uint64(3601)))
			Expect(sut.tree.CheckInvariants()).To(Succeed())
		})

		It("costs only the corrected leaf's own root to leaf path", func() {
			sut.forEmptyTree()
			sut.PutRange(anchorSec, 3600)
			before := sut.tree.NodeCount()

			sut.Put(anchorSec+1800, 9_000_000)

			Expect(sut.tree.NodeCount()).To(Equal(before))
		})
	})

	Context("retention", func() {
		It("leaves the queried window untouched", func() {
			sut.forEmptyTree()
			sut.PutRange(anchorSec, 7200)
			expected := sut.AggregateRange(anchorSec+3600, anchorSec+7199)

			sut.tree.EvictBefore(anchorSec + 3600)

			Expect(sut.AggregateRange(anchorSec+3600, anchorSec+7199).Equal(expected)).To(BeTrue())
		})

		It("reclaims the nodes the evicted leaves leave behind", func() {
			sut.forEmptyTree()
			sut.PutRange(anchorSec, 7200)

			sut.tree.EvictBefore(anchorSec + 3600)

			Expect(sut.AggregateRange(0, ^uint64(0)).Count()).To(Equal(uint64(3600)))
			Expect(sut.tree.NodeCount()).To(BeNumerically("<", 7200))
			Expect(sut.tree.CheckInvariants()).To(Succeed())
		})

		It("empties the tree when everything expires", func() {
			sut.forEmptyTree()
			sut.PutRange(anchorSec, 100)

			sut.tree.EvictBefore(anchorSec + 100)

			Expect(sut.tree.NodeCount()).To(BeZero())
			Expect(sut.AggregateRange(0, ^uint64(0)).Count()).To(BeZero())
		})

		It("keeps memory flat over a long run", func() {
			sut.forEmptyTree()
			var peak int
			for offset := range uint64(20_000) {
				sec := anchorSec + offset
				sut.Put(sec, 4_000_000)
				if sec > 3600 {
					sut.tree.EvictBefore(sec - 3600)
				}
				if offset > 5000 {
					peak = max(peak, sut.tree.NodeCount())
				}
			}

			Expect(peak).To(BeNumerically("<", 4200))
			Expect(sut.tree.CheckInvariants()).To(Succeed())
		})
	})
})

type treeSut struct {
	tree *radixtree.Tree[*radixtree.Tally]
}

func (s *treeSut) forEmptyTree() {
	s.tree = radixtree.New(radixtree.NewTally)
}

func (s *treeSut) Put(sec uint64, value uint64) {
	s.tree.Update(sec, radixtree.TallyOf(value))
}

func (s *treeSut) PutRange(from uint64, seconds int) {
	for i := range seconds {
		s.Put(from+uint64(i), 1_000_000+uint64(i)*1000)
	}
}

func (s *treeSut) AggregateRange(from, to uint64) *radixtree.Tally {
	return s.tree.AggregateRange(from, to)
}

// naiveFold merges one second at a time, which is what the range decomposition
// has to agree with.
func (s *treeSut) naiveFold(from, to uint64) *radixtree.Tally {
	acc := radixtree.NewTally()
	for sec := from; sec <= to; sec++ {
		acc.Merge(s.tree.AggregateRange(sec, sec))
	}
	return acc
}
