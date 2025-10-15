package servicelevels

import (
	"hotline/metrics"
	"time"
)

type LatencySLO struct {
	window      *metrics.SlidingWindow[float64]
	percentiles []PercentileDefinition
	namespace   string
	tags        map[string]string
	createdAt   time.Time
}

type PercentileDefinition struct {
	Percentile Percentile
	Threshold  LatencyMs
	Name       string
}

func NewLatencySLO(
	percentiles []PercentileDefinition,
	windowDuration time.Duration,
	namespace string,
	tags map[string]string,
	createdAt time.Time,
) *LatencySLO {
	var splitLatencies []float64
	for i := range percentiles {
		splitLatencies = append(splitLatencies, float64(percentiles[i].Threshold))
	}
	window := metrics.NewSlidingWindow(func() metrics.Accumulator[float64] {
		return metrics.NewLatencyHistogram(splitLatencies)
	}, windowDuration, 10*time.Second)

	return &LatencySLO{
		percentiles: percentiles,
		window:      window,
		namespace:   namespace,
		tags:        tags,
		createdAt:   createdAt,
	}
}

func (s *LatencySLO) Check(now time.Time) []LevelsCheck {
	activeWindow := s.window.GetActiveWindow(now)
	if activeWindow == nil {
		return nil
	}
	uptime := now.Sub(s.createdAt)

	histogram := activeWindow.Accumulator.(*metrics.LatencyHistogram)
	levels := make([]LevelsCheck, len(s.percentiles))
	for i, definition := range s.percentiles {
		bucket, eventsCount := histogram.ComputePercentile(definition.Percentile.Normalized())
		metric := bucket.To

		var breach *SLOBreach
		if !(metric < float64(definition.Threshold)) {
			breach = &SLOBreach{
				ThresholdValue: float64(definition.Threshold),
				ThresholdUnit:  "ms",
				Operation:      OperationL,
				WindowDuration: s.window.Size,
			}
		}
		levels[i] = LevelsCheck{
			Namespace: s.namespace,
			Metric: Metric{
				Name:        definition.Name,
				Value:       metric,
				Unit:        "ms",
				EventsCount: eventsCount,
			},
			Tags:      s.tags,
			Breach:    breach,
			Timestamp: now,
			Uptime:    uptime,
		}
	}
	return levels
}

func (s *LatencySLO) AddLatency(now time.Time, latency LatencyMs) {
	s.window.AddValue(now, float64(latency))
}
