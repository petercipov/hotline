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
	counter        int64
	splitThreshold *float64
	splitCounter   int64
}

func (b *bucketCounter) Inc(latency float64) {
	if b.splitThreshold != nil && latency <= *b.splitThreshold {
		b.splitCounter++
	} else {
		b.counter++
	}
}

func (b *bucketCounter) Sum() int64 {
	return b.counter + b.splitCounter
}

func (b *bucketCounter) Split(toDistribute int64, maxTo float64) float64 {
	if toDistribute >= b.splitCounter {
		return maxTo
	}
	return *b.splitThreshold
}

type Histogram struct {
	buckets map[bucketIndex]bucketCounter
	layout  *exponentialBucketLayout
}

func NewHistogram(splitThreshold *float64) *Histogram {
	h := &Histogram{
		buckets: make(map[bucketIndex]bucketCounter),
		layout:  newExponentialLayout(),
	}

	if splitThreshold != nil {
		key := h.layout.key(*splitThreshold)
		h.buckets[key] = bucketCounter{
			key:            key,
			counter:        0,
			splitThreshold: splitThreshold,
			splitCounter:   0,
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
	to := bucket.Split(toDistribute, h.layout.to(index))
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
		bucket = bucketCounter{key: key}
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
