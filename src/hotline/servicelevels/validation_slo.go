package servicelevels

import (
	"hotline/metrics"
	"time"
)

type ValidationSLO struct {
	window    *metrics.SlidingWindow[string]
	namespace string
	tags      map[string]string
}

func NewValidationSLO(
	windowDuration time.Duration,
	namespace string,
	tags map[string]string,
) *ValidationSLO {
	window := metrics.NewSlidingWindow(func() metrics.Accumulator[string] {
		return metrics.NewTagsHistogram([]string{"skipped"})
	}, windowDuration, 1*time.Minute)

	return &ValidationSLO{
		window:    window,
		namespace: namespace,
		tags:      tags,
	}
}

func (s *ValidationSLO) AddValidation(now time.Time) {
	s.window.AddValue(now, "skipped")
}

func (s *ValidationSLO) Check(now time.Time) []LevelsCheck {
	activeWindow := s.window.GetActiveWindow(now)
	if activeWindow == nil {
		return nil
	}
	checks := make([]LevelsCheck, 1)
	checks = checks[:0]
	histogram := activeWindow.Accumulator.(*metrics.TagHistogram)

	percentage, total := histogram.ComputePercentile("skipped")
	if percentage != nil {
		checks = append(checks, LevelsCheck{
			Namespace: s.namespace,
			Timestamp: now,
			Metric: Metric{
				Name:        "skipped",
				Value:       *percentage,
				Unit:        "%",
				EventsCount: total,
			},
			Breakdown: nil,
			Breach:    nil,
			Tags:      s.tags,
		})
	}

	return checks
}
