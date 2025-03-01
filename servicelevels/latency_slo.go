package servicelevels

import "time"

type LatencySLO struct {
	window      *SlidingWindow
	percentiles []float64
}

func NewLatencySLO(percentiles []float64, windowDuration time.Duration, splitLatencies []float64) *LatencySLO {
	window := NewSlidingWindow(func() Accumulator {
		return NewLatencyHistogram(splitLatencies)
	}, windowDuration, 10*time.Second)
	return &LatencySLO{
		percentiles: percentiles,
		window:      window,
	}
}

func (s *LatencySLO) GetMetrics(now time.Time) []float64 {
	activeWindow := s.window.GetActiveWindow(now)
	if activeWindow == nil {
		return make([]float64, len(s.percentiles))
	}

	histogram := activeWindow.Accumulator.(*LatencyHistogram)
	metrics := make([]float64, len(s.percentiles))
	for i, percentile := range s.percentiles {
		metric := histogram.ComputePercentile(percentile).To
		metrics[i] = metric
	}
	return metrics
}

func (s *LatencySLO) AddLatency(now time.Time, latency float64) {
	s.window.AddValue(now, latency)
}
