# t-digest under the radix tree

Why `radixtree` demands a commutative monoid from its payload, why t-digest is
not one, and what that costs — including the implementation Redis ships.

## What the tree requires

`radixtree.Tree[T]` is a cache of pre-folded partial results arranged by time.
The only operation it performs on a payload is `Merge`, and `Factory[T]`
supplies the identity element. That is the whole contract in
[`mergeable.go`](../src/hotline/metrics/radixtree/mergeable.go):

> Anything that is a commutative monoid under Merge can be indexed by it.

The word *commutative* is not decoration. Three things in the tree fold the
same measurements in different groupings and different orders:

**Range decomposition.** `AggregateRange(from, to)` merges one precomputed
aggregate for every block that lies entirely inside the range and descends only
at the two boundaries. Which blocks those are depends on where the window
happens to fall against the radix-16 prefixes. The same second therefore
reaches the result through a different bracketing depending on the window it is
queried in — that is associativity, asked for at every query.

**Partials arriving in any order.** `SlidingWindowSketch.AddPartial` folds a
sketch built elsewhere into a second, and the sharded-ingestion specs merge the
same stream under 2, 3, 5, 8 and 16 shard layouts. Nothing coordinates arrival
order — that is commutativity.

**Correction and unwind.** `Update` recomputes every node aggregate on the root
to leaf path from its children rather than patching it, which is what lets the
tree carry a non-invertible operator at all. A late event re-folds its leaf's
ancestors, so those merges happen more than once over the same data.

The tree does what it can on its own: `node.fold` iterates children in fixed
slot order, so a single node's fold is reproducible even for an order-dependent
payload. It cannot fix the grouping, because the grouping is chosen by the
query.

## Why t-digest is not a monoid

A t-digest merge re-clusters centroids under a scale function. It is lossy and
it is order dependent: folding the same measurements in a different grouping or
a different order yields a different digest, so the answer can depend on how a
window decomposes into blocks and on the order partial results arrive.

This is not a suspicion about the algorithm, it is a property the suite pins.
[`digestcompare`](../src/hotline/metrics/digestcompare) replays one identical
stream through both payloads under the same tree and counts how many distinct
results 200 repartitions produce:

- DDSketch: exactly one — *"a repartition MUST NOT change a DDSketch"*
- t-digest: more than one — *"t-digest merge re-clusters, so it cannot be order
  independent"*

## The implementation Redis ships

RedisBloom vendors [RedisBloom/t-digest-c](https://github.com/RedisBloom/t-digest-c),
a fork of the C merging-digest port, behind the `TDIGEST.*` commands. It fails
both laws, and commutativity for a sharper reason than our own port does.

```c
int td_merge(td_histogram_t *into, td_histogram_t *from) {
    if (td_compress(into) != 0) return EDOM;
    if (td_compress(from) != 0) return EDOM;
    const int pos = from->merged_nodes + from->unmerged_nodes;
    for (int i = 0; i < pos; i++) {
        const double mean = from->nodes_mean[i];
        const long long weight = from->nodes_weight[i];
        if (td_add(into, mean, weight) != 0) return EDOM;
    }
    return 0;
}
```

`td_add` compresses first when the node array is nearly full
(`(merged + unmerged) >= cap - 1`), and `td_compress` sorts the combined
centroids by mean before re-clustering greedily under a limit whose
`normalizer` is `compression / (2π · total_weight · log(total_weight))`.

**Associativity fails.** Clustering happens at merge time and the boundary test
reads `total_weight` as it stands when compression fires. `(A⊕B)⊕C` and
`A⊕(B⊕C)` compress at different fill levels against different totals, so
different centroids survive.

**Commutativity fails.** `td_merge(into, from)` is asymmetric by construction:
it pours the source's centroids in one at a time while compression triggers on
the destination's fill, so `merge(A,B)` and `merge(B,A)` compress at different
points. The sort is introsort with three-way partitioning and is not stable —
the source notes the order within a run of equal keys differs from a naive
sort — so tied means can resolve by array layout.

An empty digest is still an identity and merge is closed, so what Redis offers
is a magma with identity, not a monoid. `TDIGEST.MERGE` buys accuracy, not
shard invariance.

Read from `master` of the source above rather than measured by running it. The
empirical check, if it is ever worth doing: create three sketches, merge them
in both groupings and both orders, and compare `TDIGEST.BYRANK`.

## What this costs us

- [`ddsketch`](../src/hotline/metrics/ddsketch) is the production payload. Its
  merge is per-bucket integer addition, exactly associative and commutative, so
  one fold serves per-second leaves, internal nodes, cross-process partials and
  cross-shard rollups alike.
- `SlidingWindowSketch` exists only in `ddsketch`. Its contract — out-of-order
  equivalence, identical results under any shard layout, eviction that never
  changes an answer — is exactly what a t-digest payload cannot honour, so a
  `tdigest` counterpart would publish guarantees it would break.
- [`tdigest`](../src/hotline/metrics/tdigest) stays as the control. It is still
  indexed by `radixtree.Tree[*tdigest.Sketch]`, because the tree asks only for
  `Mergeable`; the comparison uses that to quantify the divergence rather than
  assume it.
