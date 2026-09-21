package tdigest

import "math"

type WeightScaling struct {
	normalizer float64
}

func NewWeightScaling(capacity int) *WeightScaling {
	normalizer := 2 * math.Abs(math.Sin(math.Pi/float64(capacity)))
	return &WeightScaling{
		normalizer: normalizer,
	}
}

func (s *WeightScaling) MaxWeight(quantile1, quantile2 float64, totalWeight uint64) float64 {
	quantile := (quantile1 + quantile2) * 0.5
	scale := s.normalizer * math.Sqrt(quantile*(1-quantile))
	return scale*float64(totalWeight) + 1.0
}
