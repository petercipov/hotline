package tdigest

import (
	"math"
	"math/rand/v2"
	"sort"
)

// Computing Extremely Accurate Quantiles Using t-Digests
// https://arxiv.org/pdf/1902.04023

// Centroids is a list of Centroid
// Centroid is the mean of near values.
// Centroids are sorted by Mean
type Centroids struct {
	mean        []float64
	weight      []uint64
	totalWeight uint64
}

type Centroid struct {
	Mean   float64
	Weight uint64
}

type CentroidList []Centroid

func (l CentroidList) Swap(i, j int) {
	l[i], l[j] = l[j], l[i]
}

func (l CentroidList) TotalWeight() uint64 {
	totalWeight := uint64(0)
	for _, v := range l {
		totalWeight += v.Weight
	}
	return totalWeight
}

func (c *Centroids) Size() int {
	return len(c.mean)
}

func (c *Centroids) AddCentroid(mean float64, count uint64) {
	indexToAppend := sort.Search(len(c.mean), func(i int) bool {
		return c.mean[i] > mean
	})

	c.mean = append(c.mean, 0.0)
	c.weight = append(c.weight, 0)

	copy(c.mean[indexToAppend+1:], c.mean[indexToAppend:])
	copy(c.weight[indexToAppend+1:], c.weight[indexToAppend:])

	c.mean[indexToAppend] = mean
	c.weight[indexToAppend] = count
	c.totalWeight += count
}

func (c *Centroids) TotalWeight() uint64 {
	return c.totalWeight
}

func (c *Centroids) ToList() []Centroid {
	list := make([]Centroid, 0, c.Size())
	for index := range c.mean {
		list = append(list, Centroid{
			Mean:   c.mean[index],
			Weight: c.weight[index],
		})
	}
	return list
}

func (c *Centroids) UpdateCentroid(index int, mean float64, count uint64) bool {
	if index+1 > c.Size() {
		return false
	}

	oldCount := c.weight[index]
	oldMean := c.mean[index]
	newCount := oldCount + count
	newMean := (mean*float64(count) + oldMean*float64(oldCount)) / float64(newCount)
	c.mean[index] = newMean
	c.weight[index] = newCount
	c.totalWeight += count
	return true
}

func (c *Centroids) MinimumDistanceCentroid(value float64) (int, bool) {
	if len(c.mean) == 0 {
		return 0, false
	}

	var minimumDistance = math.MaxFloat64
	var minimumDistanceIndex int
	var found = false
	for index, mean := range c.mean {
		distance := math.Abs(mean - value)
		if distance <= minimumDistance {
			minimumDistanceIndex = index
			minimumDistance = distance
			found = true
		}
	}
	return minimumDistanceIndex, found
}

func (c *Centroids) ComputeCentroidQuantiles(index int) (float64, float64) {
	var weightUpToIndex uint64
	for i := range index {
		weightUpToIndex += c.weight[i]
	}
	q0 := float64(weightUpToIndex) / float64(c.totalWeight)
	q1 := float64(weightUpToIndex+c.weight[index]) / float64(c.totalWeight)
	return q0, q1
}

func (c *Centroids) CentroidAt(index int) Centroid {
	return Centroid{
		Mean:   c.mean[index],
		Weight: c.weight[index],
	}
}

func (c *Centroids) FittingCumulativeWeightCentroid(sum uint64) (uint64, int, bool) {
	if len(c.weight) == 0 {
		return 0, 0, false
	}
	if sum <= 0 {
		return c.weight[0], 0, true
	}

	startSum := uint64(0)
	endSum := uint64(0)
	for index, weight := range c.weight {
		startSum = endSum
		endSum += weight
		if sum > startSum && sum <= endSum {
			return endSum, index, true
		}
	}
	return endSum, len(c.weight) - 1, true
}

func NewCentroids(capacity int) *Centroids {
	return &Centroids{
		mean:   make([]float64, 0, capacity),
		weight: make([]uint64, 0, capacity),
	}
}

type MaxWeightFunc func(quantile1, quantile2 float64, totalWeight uint64) float64
type TDigest struct {
	centroids          *Centroids
	quantileMaxWeights MaxWeightFunc
	capacity           int
	bufferSize         int

	unprocessed CentroidList
	randomizer  *rand.Rand
}

func (d *TDigest) AddToBuffer(mean float64, weight uint64) {
	d.unprocessed = append(d.unprocessed, Centroid{
		Mean:   mean,
		Weight: weight,
	})

	if len(d.unprocessed) >= d.bufferSize {
		d.processBuffer()
	}
}

func (d *TDigest) processBuffer() {
	d.randomizer.Shuffle(len(d.unprocessed), d.unprocessed.Swap)

	d.unprocessed.TotalWeight()
	for _, centroid := range d.unprocessed {
		d.addEntry(centroid.Mean, centroid.Weight)
	}
	d.unprocessed = d.unprocessed[:0]
}

func (d *TDigest) addEntry(mean float64, weight uint64) {
	if d.centroids.Size() == 0 {
		d.centroids.AddCentroid(mean, weight)
		return
	}

	index, _ := d.centroids.MinimumDistanceCentroid(mean)
	minimumDistanceCentroid := d.centroids.CentroidAt(index)
	isSameValueCentroid := minimumDistanceCentroid.Mean == mean

	if isSameValueCentroid {
		d.centroids.UpdateCentroid(index, mean, weight)
		return
	}

	q1, q2 := d.centroids.ComputeCentroidQuantiles(index)
	weightThreshold := d.quantileMaxWeights(q1, q2, d.centroids.TotalWeight())
	underWeightThreshold := minimumDistanceCentroid.Weight+weight <= uint64(weightThreshold)
	if underWeightThreshold {
		d.centroids.UpdateCentroid(index, mean, weight)
	} else {
		d.centroids.AddCentroid(mean, weight)
	}
}

func (d *TDigest) ToCentroids() []Centroid {
	d.processBuffer()
	return d.centroids.ToList()
}

func (d *TDigest) Quantile(percentile float64) float64 {
	d.processBuffer()

	if percentile < 0 || percentile > 1 {
		return math.NaN()
	}
	if d.centroids.Size() == 0 {
		return 0
	}
	if d.centroids.Size() == 1 {
		return d.centroids.CentroidAt(0).Mean
	}
	percentileCumulativeWeight := uint64(math.Floor(float64(d.centroids.TotalWeight()) * percentile))

	fittingCumulativeWeight, centroidIndex, _ := d.centroids.FittingCumulativeWeightCentroid(percentileCumulativeWeight)
	fittingCentroid := d.centroids.CentroidAt(centroidIndex)

	hasPreviousCentroid := centroidIndex > 0

	if !hasPreviousCentroid {
		return d.centroids.CentroidAt(0).Mean
	}

	previousCentroid := d.centroids.CentroidAt(centroidIndex - 1)
	previousCumulativeWeight := fittingCumulativeWeight - fittingCentroid.Weight

	growthFactor := float64(percentileCumulativeWeight-previousCumulativeWeight) /
		float64(fittingCumulativeWeight-previousCumulativeWeight)

	linearlyApproximateQuantile := previousCentroid.Mean + (growthFactor * (fittingCentroid.Mean - previousCentroid.Mean))

	return linearlyApproximateQuantile
}

func NewTDigestWeightScaled(capacity int, bufferSize int, randomizer *rand.Rand) *TDigest {
	centroids := NewCentroids(capacity)
	scaling := NewWeightScaling(capacity)
	return &TDigest{
		capacity:           capacity,
		centroids:          centroids,
		quantileMaxWeights: scaling.MaxWeight,
		unprocessed:        make(CentroidList, 0, bufferSize),
		randomizer:         randomizer,
		bufferSize:         bufferSize,
	}
}
