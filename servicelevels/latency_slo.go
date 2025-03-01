package servicelevels

import (
	"fmt"
	"time"
)

type LatencySLO struct {
	window          *SlidingWindow[float64]
	percentiles     []float64
	percentileNames []string
}

func NewLatencySLO(percentiles []float64, windowDuration time.Duration, splitLatencies []float64) *LatencySLO {
	window := NewSlidingWindow(func() Accumulator[float64] {
		return NewLatencyHistogram(splitLatencies)
	}, windowDuration, 10*time.Second)

	percentileNames := make([]string, len(percentiles))
	for i, p := range percentiles {
		percentileNames[i] = fmt.Sprintf("p%g", p*100)
	}

	return &LatencySLO{
		percentiles:     percentiles,
		percentileNames: percentileNames,
		window:          window,
	}
}

func (s *LatencySLO) Check(now time.Time) []SLOCheck {
	activeWindow := s.window.GetActiveWindow(now)
	if activeWindow == nil {
		return make([]SLOCheck, len(s.percentiles))
	}

	histogram := activeWindow.Accumulator.(*LatencyHistogram)
	metrics := make([]SLOCheck, len(s.percentiles))
	for i, percentile := range s.percentiles {
		metric := histogram.ComputePercentile(percentile).To
		metrics[i] = SLOCheck{
			MetricValue: metric,
			MetricName:  s.percentileNames[i],
		}
	}
	return metrics
}

func (s *LatencySLO) AddLatency(now time.Time, latency float64) {
	s.window.AddValue(now, latency)
}
