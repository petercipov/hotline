package servicelevels

import (
	"maps"
	"math"
	"slices"
	"unsafe"
)

type Histogram struct {
	counters map[bucketIndex]int64
}

func NewHistogram() *Histogram {
	return &Histogram{
		counters: make(map[bucketIndex]int64),
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
		From: index.from(),
		To:   index.to(),
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
	key := exponentiallyDistributedKey(latency)
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

const growthFactor = 1.15

var growthDivisor = math.Log(growthFactor)

type bucketIndex int

func exponentiallyDistributedKey(latency float64) bucketIndex {
	return bucketIndex(math.Floor(math.Log(latency) / growthDivisor))
}

func (h *bucketIndex) to() float64 {
	return math.Pow(growthFactor, float64(*h+1))
}

func (h *bucketIndex) from() float64 {
	if *h == 0 {
		return 0
	}

	return math.Pow(growthFactor, float64(*h))
}
