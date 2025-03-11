package servicelevels

import (
	"time"
)

type LatencySLO struct {
	window      *SlidingWindow[float64]
	percentiles []PercentileDefinition
	tags        map[string]string
}

type PercentileDefinition struct {
	Percentile Percentile
	Threshold  LatencyMs
	Name       string
}

func NewLatencySLO(percentiles []PercentileDefinition, windowDuration time.Duration, tags map[string]string) *LatencySLO {
	var splitLatencies []float64
	for i := range percentiles {
		splitLatencies = append(splitLatencies, float64(percentiles[i].Threshold))
	}
	window := NewSlidingWindow(func() Accumulator[float64] {
		return NewLatencyHistogram(splitLatencies)
	}, windowDuration, 10*time.Second)

	return &LatencySLO{
		percentiles: percentiles,
		window:      window,
		tags:        tags,
	}
}

func (s *LatencySLO) Check(now time.Time) []SLOCheck {
	activeWindow := s.window.GetActiveWindow(now)
	if activeWindow == nil {
		return nil
	}

	histogram := activeWindow.Accumulator.(*LatencyHistogram)
	metrics := make([]SLOCheck, len(s.percentiles))
	for i, definition := range s.percentiles {
		metric := histogram.ComputePercentile(definition.Percentile.Normalized()).To

		var breach *SLOBreach
		if !(metric < float64(definition.Threshold)) {
			breach = &SLOBreach{
				ThresholdValue: float64(definition.Threshold),
				ThresholdUnit:  "ms",
				Operation:      OperationL,
				WindowDuration: s.window.Size,
			}
		}
		metrics[i] = SLOCheck{
			Metric: Metric{
				Name:  definition.Name,
				Value: metric,
			},
			Tags:   s.tags,
			Breach: breach,
		}
	}
	return metrics
}

func (s *LatencySLO) AddLatency(now time.Time, latency LatencyMs) {
	s.window.AddValue(now, float64(latency))
}
