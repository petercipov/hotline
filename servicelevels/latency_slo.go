package servicelevels

import (
	"time"
)

type LatencySLO struct {
	window      *SlidingWindow[float64]
	percentiles []PercentileDefinition
}

type PercentileDefinition struct {
	Percentile Percentile
	Threshold  LatencyMs
	Name       string
}

func NewLatencySLO(percentiles []PercentileDefinition, windowDuration time.Duration) *LatencySLO {
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
	}
}

func (s *LatencySLO) Check(now time.Time) []SLOCheck {
	activeWindow := s.window.GetActiveWindow(now)
	if activeWindow == nil {
		return make([]SLOCheck, len(s.percentiles))
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
			Breach: breach,
		}
	}
	return metrics
}

func (s *LatencySLO) AddLatency(now time.Time, latency LatencyMs) {
	s.window.AddValue(now, float64(latency))
}
