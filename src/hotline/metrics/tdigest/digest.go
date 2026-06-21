package tdigest

import (
	"math"
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

func (c *Centroid) UpdateCentroid(mean float64, weight uint64) {
	newWeight := c.Weight + weight
	c.Mean = (c.Mean*float64(c.Weight) + mean*float64(weight)) / float64(newWeight)
	c.Weight = newWeight
}

type CentroidBuffer []Centroid

func (l CentroidBuffer) Swap(i, j int) {
	l[i], l[j] = l[j], l[i]
}

func (l CentroidBuffer) TotalWeight() uint64 {
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

	unprocessed CentroidBuffer
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
	if len(d.unprocessed) == 0 {
		return
	}

	d.centroids = d.greedyCompress(append(d.centroids.ToList(), d.unprocessed...))
	d.unprocessed = d.unprocessed[:0]
}

func (d *TDigest) greedyCompress(values CentroidBuffer) *Centroids {
	if len(values) == 0 {
		return NewCentroids(d.capacity)
	}
	sort.Slice(values, func(i, j int) bool {
		return values[i].Mean < values[j].Mean
	})

	totalWeight := values.TotalWeight()
	compressed := NewCentroids(d.capacity)
	current := values[0]
	cumulativeWeight := uint64(0)

	for _, candidate := range values[1:] {
		q0 := float64(cumulativeWeight) / float64(totalWeight)
		q2 := float64(cumulativeWeight+current.Weight+candidate.Weight) / float64(totalWeight)
		weightThreshold := d.quantileMaxWeights(q0, q2, totalWeight)

		if current.Mean == candidate.Mean || float64(current.Weight+candidate.Weight) <= weightThreshold {
			current.UpdateCentroid(candidate.Mean, candidate.Weight)
			continue
		}

		compressed.AddCentroid(current.Mean, current.Weight)
		cumulativeWeight += current.Weight
		current = candidate
	}

	compressed.AddCentroid(current.Mean, current.Weight)
	return compressed
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

func NewTDigestWeightScaled(capacity int, bufferSize int) *TDigest {
	centroids := NewCentroids(capacity)
	scaling := NewWeightScaling(capacity)
	return &TDigest{
		capacity:           capacity,
		centroids:          centroids,
		quantileMaxWeights: scaling.MaxWeight,
		unprocessed:        make(CentroidBuffer, 0, bufferSize),
		bufferSize:         bufferSize,
	}
}
