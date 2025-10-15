package metrics

type TagHistogram[T comparable] struct {
	buckets *bucketedCounters
	layout  *tagsLayout[T]
}

func NewTagsHistogram[T comparable](tags []T) *TagHistogram[T] {
	return &TagHistogram[T]{
		buckets: newBucketedCounters(),
		layout:  newTagsLayout(tags),
	}
}

func (h *TagHistogram[T]) Add(tag T) {
	key := h.layout.key(tag)
	if key == nil {
		return
	}
	h.buckets.Add(*key, 1)
}

func (h *TagHistogram[T]) ComputePercentile(tag T) (*float64, int64) {
	key := h.layout.key(tag)
	if key == nil {
		return nil, 0
	}
	total := float64(h.buckets.Sum())
	counter := h.buckets.GetCounter(*key)
	if counter == nil {
		return nil, 0
	}
	sum := counter.Sum()
	percentile := float64(sum) / total * 100.0
	return &percentile, sum
}

type tagsLayout[T comparable] struct {
	tags              []T
	toIndex           map[T]*bucketIndex
	indexedByTagOrder []bucketIndex
}

func newTagsLayout[T comparable](tags []T) *tagsLayout[T] {
	toIndex := make(map[T]*bucketIndex, len(tags))
	sortedIndexes := make([]bucketIndex, len(tags))
	for i, tag := range tags {
		index := bucketIndex(i)
		toIndex[tag] = &index
		sortedIndexes[i] = index
	}
	return &tagsLayout[T]{
		tags:              tags,
		toIndex:           toIndex,
		indexedByTagOrder: sortedIndexes,
	}
}

func (l *tagsLayout[T]) key(tag T) *bucketIndex {
	key, found := l.toIndex[tag]
	if !found {
		return nil
	}
	return key
}
