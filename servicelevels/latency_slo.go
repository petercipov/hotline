package servicelevels

import "time"

type LatencySLO struct {
	window     *SlidingWindow
	percentile float64
}

func NewLatencySLO(percentile float64, windowDuration time.Duration, splitLatencies []float64) *LatencySLO {
	window := NewSlidingWindow(func() Accumulator {
		return NewHistogram(splitLatencies)
	}, windowDuration, 10*time.Second)
	return &LatencySLO{
		percentile: percentile,
		window:     window,
	}
}

func (s *LatencySLO) GetMetric(now time.Time) float64 {
	activeWindow := s.window.GetActiveWindow(now)
	if activeWindow == nil {
		return 0
	}

	histogram := activeWindow.Accumulator.(*Histogram)
	bucket := histogram.ComputePercentile(s.percentile)
	return bucket.To
}

func (s *LatencySLO) AddLatency(now time.Time, latency float64) {
	s.window.AddValue(now, latency)
}
