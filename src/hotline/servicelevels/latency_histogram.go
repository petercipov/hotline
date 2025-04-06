package servicelevels

import (
	"math"
	"slices"
	"unsafe"
)

type LatencyHistogram struct {
	buckets     *bucketedCounters
	layout      *exponentialBucketLayout
	splitLength int
}

func NewLatencyHistogram(splitLatencies []float64) *LatencyHistogram {
	h := &LatencyHistogram{
		buckets:     newBucketedCounters(),
		layout:      newExponentialLayout(),
		splitLength: len(splitLatencies),
	}
	slices.Sort(splitLatencies)
	latenciesByKey := make(map[bucketIndex][]float64)
	for _, splitLatency := range splitLatencies {
		key := h.layout.key(splitLatency)
		latenciesForKey, found := latenciesByKey[key]
		if !found {
			latenciesForKey = []float64{splitLatency}
		} else {
			latenciesForKey = append(latenciesForKey, splitLatency)
		}
		latenciesByKey[key] = latenciesForKey
	}

	for key, latenciesForKey := range latenciesByKey {
		h.buckets.CreateSplit(key, latenciesForKey)
	}
	return h
}

type Bucket struct {
	From float64
	To   float64
}

func (h *LatencyHistogram) ComputePercentile(percentile float64) (Bucket, int64) {
	count := h.buckets.Sum()
	if count <= 2 {
		return Bucket{}, count
	}

	pThreshold := int64(math.Ceil(float64(count) * percentile))
	index, toDistributeInsideBucket := h.findFirstBucketOverThreshold(pThreshold)

	bucket := h.buckets.GetCounter(index)
	split := bucket.Split(toDistributeInsideBucket)
	var to float64
	if split == nil {
		to = h.layout.bucketTo(index)
	} else {
		to = *split
	}
	return Bucket{
			From: h.layout.bucketFrom(index),
			To:   to,
		},
		count
}

func (h *LatencyHistogram) findFirstBucketOverThreshold(threshold int64) (bucketIndex, int64) {
	entries := int64(0)
	sortedKeys := h.buckets.GetSortedIndexes()
	firstBucketIndexOverThreshold := sortedKeys[len(sortedKeys)-1]
	toDistributeInsideBucket := int64(0)
	if len(sortedKeys) == 1 {
		return firstBucketIndexOverThreshold, toDistributeInsideBucket
	}

	for _, sortedKey := range sortedKeys {
		bucket := h.buckets.GetCounter(sortedKey)
		bucketSum := bucket.Sum()
		if entries+bucketSum >= threshold {
			firstBucketIndexOverThreshold = sortedKey
			toDistributeInsideBucket = threshold - entries
			break
		} else {
			entries += bucketSum
		}
	}
	return firstBucketIndexOverThreshold, toDistributeInsideBucket
}

func (h *LatencyHistogram) Add(latency float64) {
	key := h.layout.key(latency)
	h.buckets.Add(key, latency)
}

func (h *LatencyHistogram) SizeInBytes() int {
	sizeOfSplit := int(unsafe.Sizeof(&splitCounter{}))
	h.buckets.SizeInBytes()
	return h.buckets.SizeInBytes() +
		(h.splitLength * sizeOfSplit)
}

type exponentialBucketLayout struct {
	growthFactor        float64
	growthDivisor       float64
	zeroBucketThreshold float64
}

func newExponentialLayout() *exponentialBucketLayout {
	growthFactor := 1.15
	return &exponentialBucketLayout{
		growthFactor:        growthFactor,
		growthDivisor:       math.Log(growthFactor),
		zeroBucketThreshold: 1.0,
	}
}

func (l *exponentialBucketLayout) key(latency float64) bucketIndex {
	if latency < l.zeroBucketThreshold {
		return bucketIndex(0)
	}
	return bucketIndex(math.Floor(math.Log(latency) / l.growthDivisor))
}

func (l *exponentialBucketLayout) bucketFrom(index bucketIndex) float64 {
	if index == 0 {
		return 0
	}
	return math.Pow(l.growthFactor, float64(index))
}

func (l *exponentialBucketLayout) bucketTo(index bucketIndex) float64 {
	if index == 0 {
		return 1
	}
	return math.Pow(l.growthFactor, float64(index+1))
}
