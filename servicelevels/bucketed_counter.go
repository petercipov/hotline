package servicelevels

import (
	"maps"
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
	counter int64
	splits  []splitCounter
}
type bucketedCounters struct {
	buckets map[bucketIndex]*bucketCounter
}

func (c *bucketedCounters) Add(key bucketIndex, latency float64) {
	bucket, found := c.buckets[key]
	if !found {
		bucket = &bucketCounter{
			key: key,
		}
		c.buckets[key] = bucket
	}
	bucket.Inc(latency)
}

func (c *bucketedCounters) SizeInBytes() int {
	sizeOfBuckets := len(c.buckets)
	sizeOfBucket := int(unsafe.Sizeof(&bucketCounter{}))

	k := bucketIndex(0)
	sizeOfKey := int(unsafe.Sizeof(&k))

	return (sizeOfBucket * sizeOfKey) +
		(sizeOfBucket * sizeOfBuckets)
}

func (c *bucketedCounters) allCountersSum() int64 {
	sum := int64(0)
	for _, bucket := range c.buckets {
		sum += bucket.Sum()
	}
	return sum
}

func (c *bucketedCounters) createSplit(key bucketIndex, splits []float64) {
	splitCounters := make([]splitCounter, len(splits))
	for i, latencyForKey := range splits {
		splitCounters[i] = splitCounter{
			latency: latencyForKey,
			counter: 0,
		}
	}
	c.buckets[key] = &bucketCounter{
		key:     key,
		counter: 0,
		splits:  splitCounters,
	}
}

func (c *bucketedCounters) getCounter(index bucketIndex) *bucketCounter {
	return c.buckets[index]
}

func (c *bucketedCounters) getSortedIndexes() []bucketIndex {
	return slices.SortedFunc(maps.Keys(c.buckets), func(index bucketIndex, index2 bucketIndex) int {
		return int(index) - int(index2)
	})
}

func newBucketedCounters() *bucketedCounters {
	return &bucketedCounters{
		buckets: make(map[bucketIndex]*bucketCounter),
	}
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

func (b *bucketCounter) Split(toDistribute int64) *float64 {
	added := int64(0)
	for _, split := range b.splits {
		added += split.counter
		if added >= toDistribute {
			return &split.latency
		}
	}
	return nil
}
