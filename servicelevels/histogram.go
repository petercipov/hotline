package servicelevels

import (
	"maps"
	"math"
	"slices"
	"unsafe"
)

type Histogram struct {
	counters map[bucketIndex]int64
	layout   *exponentialBucketLayout
}

func NewHistogram() *Histogram {
	return &Histogram{
		counters: make(map[bucketIndex]int64),
		layout:   newExponentialLayout(),
	}
}

type Bucket struct {
	From float64
	To   float64
}

func (h *Histogram) ComputeP50() Bucket {
	count := h.counterSum()
	if count <= 2 {
		return Bucket{}
	}

	entriesThreshold := int64(math.Ceil(float64(count) * 0.5))
	index := h.findBucketGeThreshold(entriesThreshold)

	return Bucket{
		From: h.layout.from(index),
		To:   h.layout.to(index),
	}
}

func (h *Histogram) findBucketGeThreshold(threshold int64) bucketIndex {
	sortedKeys := slices.SortedFunc(maps.Keys(h.counters), func(index bucketIndex, index2 bucketIndex) int {
		return int(index) - int(index2)
	})
	entries := int64(0)

	if len(sortedKeys) == 1 {
		return sortedKeys[0]
	}

	for _, sortedKey := range sortedKeys {
		entries += h.counters[sortedKey]
		if entries >= threshold {
			return sortedKey
		}
	}
	return sortedKeys[len(sortedKeys)-1]
}

func (h *Histogram) Add(latency float64) {
	key := h.layout.key(latency)
	counter, found := h.counters[key]
	if !found {
		h.counters[key] = 1
	} else {
		h.counters[key] = counter + 1
	}
}

func (h *Histogram) counterSum() int64 {
	sum := int64(0)

	for _, counter := range h.counters {
		sum += counter
	}
	return sum
}

func (h *Histogram) SizeInBytes() int {
	b := int64(0)
	sizeOfBuckets := len(h.counters)
	sizeOfBucket := int(unsafe.Sizeof(&b))

	k := bucketIndex(0)
	sizeOfKey := int(unsafe.Sizeof(&k))

	return (sizeOfBucket * sizeOfKey) + (sizeOfBucket * sizeOfBuckets)
}

type bucketIndex int

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
