package servicelevels

import (
	"maps"
	"math"
	"slices"
	"unsafe"
)

type bucketIndex int
type bucketCounter struct {
	key            bucketIndex
	keyTo          float64
	counter        int64
	splitLatencies []float64
	splitCounters  []int64
}

func (b *bucketCounter) Inc(latency float64) {
	for i, threshold := range b.splitLatencies {
		if latency <= threshold {
			b.splitCounters[i]++
			return
		}
	}
	b.counter++
}

func (b *bucketCounter) Sum() int64 {
	sum := b.counter
	for _, splitCounter := range b.splitCounters {
		sum += splitCounter
	}
	return sum
}

func (b *bucketCounter) Split(toDistribute int64) float64 {
	added := int64(0)
	for i, splitCounter := range b.splitCounters {
		added += splitCounter
		if added >= toDistribute {
			return b.splitLatencies[i]
		}
	}
	return b.keyTo
}

type Histogram struct {
	buckets map[bucketIndex]bucketCounter
	layout  *exponentialBucketLayout
}

func NewHistogram(splitLatencies []float64) *Histogram {
	h := &Histogram{
		buckets: make(map[bucketIndex]bucketCounter),
		layout:  newExponentialLayout(),
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
		h.buckets[key] = bucketCounter{
			key:            key,
			keyTo:          h.layout.to(key),
			counter:        0,
			splitLatencies: latenciesForKey,
			splitCounters:  make([]int64, len(latenciesForKey)),
		}
	}
	return h
}

type Bucket struct {
	From float64
	To   float64
}

func (h *Histogram) ComputeP50() Bucket {
	count := h.allCountersSum()
	if count <= 2 {
		return Bucket{}
	}

	pThreshold := int64(math.Ceil(float64(count) * 0.5))
	index, toDistribute := h.findBucketGeThreshold(pThreshold)

	bucket := h.buckets[index]
	to := bucket.Split(toDistribute)
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
	b := bucketCounter{}
	sizeOfBuckets := len(h.buckets)
	sizeOfBucket := int(unsafe.Sizeof(&b))

	k := bucketIndex(0)
	sizeOfKey := int(unsafe.Sizeof(&k))

	return (sizeOfBucket * sizeOfKey) + (sizeOfBucket * sizeOfBuckets)
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
