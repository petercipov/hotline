package servicelevels

type TagHistogram struct {
	buckets *bucketedCounters
	layout  *tagsLayout
}

func NewTagsHistogram(tags []string) *TagHistogram {
	return &TagHistogram{
		buckets: newBucketedCounters(),
		layout:  newTagsLayout(tags),
	}
}

func (h *TagHistogram) Add(tag string) {
	key := h.layout.key(tag)
	if key == nil {
		return
	}
	h.buckets.Add(*key, 1)
}

func (h *TagHistogram) ComputePercentile(tag string) (*float64, int64) {
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

type tagsLayout struct {
	tags              []string
	toIndex           map[string]*bucketIndex
	indexedByTagOrder []bucketIndex
}

func newTagsLayout(tags []string) *tagsLayout {
	toIndex := make(map[string]*bucketIndex, len(tags))
	sortedIndexes := make([]bucketIndex, len(tags))
	for i, tag := range tags {
		index := bucketIndex(i)
		toIndex[tag] = &index
		sortedIndexes[i] = index
	}
	return &tagsLayout{
		tags:              tags,
		toIndex:           toIndex,
		indexedByTagOrder: sortedIndexes,
	}
}

func (l *tagsLayout) key(tag string) *bucketIndex {
	key, found := l.toIndex[tag]
	if !found {
		return nil
	}
	return key
}
