package servicelevels

import (
	"maps"
	"math"
	"slices"
	"unsafe"
)

type bucketIndex int

type splitCounter struct {
	latency float64
	counter int64
}
type bucketCounter struct {
	key     bucketIndex
	keyTo   float64
	counter int64
	splits  []splitCounter
}

func (b *bucketCounter) Inc(latency float64) {
	for i, split := range b.splits {
		if latency <= split.latency {
			b.splits[i].counter++
			return
		}
	}
	b.counter++
}

func (b *bucketCounter) Sum() int64 {
	sum := b.counter
	for _, split := range b.splits {
		sum += split.counter
	}
	return sum
}

func (b *bucketCounter) Split(toDistribute int64) float64 {
	added := int64(0)
	for _, split := range b.splits {
		added += split.counter
		if added >= toDistribute {
			return split.latency
		}
	}
	return b.keyTo
}

type Histogram struct {
	buckets     map[bucketIndex]bucketCounter
	layout      *exponentialBucketLayout
	splitLength int
}

func NewHistogram(splitLatencies []float64) *Histogram {
	h := &Histogram{
		buckets:     make(map[bucketIndex]bucketCounter),
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
		var splits []splitCounter
		for _, latencyForKey := range latenciesForKey {
			splits = append(splits, splitCounter{
				latency: latencyForKey,
				counter: 0,
			})
		}

		h.buckets[key] = bucketCounter{
			key:     key,
			keyTo:   h.layout.to(key),
			counter: 0,
			splits:  splits,
		}
	}
	return h
}

type Bucket struct {
	From float64
	To   float64
}

func (h *Histogram) ComputePercentile(percentile float64) Bucket {
	count := h.allCountersSum()
	if count <= 2 {
		return Bucket{}
	}

	pThreshold := int64(math.Ceil(float64(count) * percentile))
	index, toDistributeInsideBucket := h.findBucketGeThreshold(pThreshold)

	bucket := h.buckets[index]
	to := bucket.Split(toDistributeInsideBucket)
	return Bucket{
		From: h.layout.from(index),
		To:   to,
	}
}

func (h *Histogram) findBucketGeThreshold(threshold int64) (bucketIndex, int64) {
	sortedKeys := slices.SortedFunc(maps.Keys(h.buckets), func(index bucketIndex, index2 bucketIndex) int {
		return int(index) - int(index2)
	})
	entries := int64(0)
	if len(sortedKeys) == 1 {
		return sortedKeys[0], 0
	}

	for _, sortedKey := range sortedKeys {
		bucket := h.buckets[sortedKey]
		bucketSum := bucket.Sum()
		if entries+bucketSum >= threshold {
			return sortedKey, threshold - entries
		} else {
			entries += bucketSum
		}
	}
	return sortedKeys[len(sortedKeys)-1], 0
}

func (h *Histogram) Add(latency float64) {
	key := h.layout.key(latency)
	bucket, found := h.buckets[key]
	if !found {
		bucket = bucketCounter{
			key:   key,
			keyTo: h.layout.to(key),
		}
	}
	bucket.Inc(latency)
	h.buckets[key] = bucket
}

func (h *Histogram) allCountersSum() int64 {
	sum := int64(0)
	for _, bucket := range h.buckets {
		sum += bucket.Sum()
	}
	return sum
}

func (h *Histogram) SizeInBytes() int {
	c := splitCounter{}
	sizeOfSplit := int(unsafe.Sizeof(&c))

	b := bucketCounter{}
	sizeOfBuckets := len(h.buckets)
	sizeOfBucket := int(unsafe.Sizeof(&b))

	k := bucketIndex(0)
	sizeOfKey := int(unsafe.Sizeof(&k))

	return (sizeOfBucket * sizeOfKey) +
		(sizeOfBucket * sizeOfBuckets) +
		(h.splitLength * sizeOfSplit)
}

type exponentialBucketLayout struct {
	growthFactor  float64
	growthDivisor float64
}

func newExponentialLayout() *exponentialBucketLayout {
	return &exponentialBucketLayout{
		growthFactor:  1.15,
		growthDivisor: math.Log(1.15),
	}
}

func (l *exponentialBucketLayout) key(latency float64) bucketIndex {
	return bucketIndex(math.Floor(math.Log(latency) / l.growthDivisor))
}

func (l *exponentialBucketLayout) from(index bucketIndex) float64 {
	if index == 0 {
		return 0
	}
	return math.Pow(l.growthFactor, float64(index))
}

func (l *exponentialBucketLayout) to(index bucketIndex) float64 {
	return math.Pow(l.growthFactor, float64(index+1))
}
