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

func (h *TagHistogram) GetPercentiles() []float64 {

	total := float64(h.buckets.Sum())
	sortedIndexes := h.layout.indexedByTagOrder
	if len(sortedIndexes) == 0 {
		return nil
	}
	percentiles := make([]float64, len(sortedIndexes))

	for i, key := range sortedIndexes {
		sum := h.buckets.GetCounter(key).Sum()
		percentile := float64(sum) / total * 100.0
		percentiles[i] = percentile
	}

	return percentiles
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
