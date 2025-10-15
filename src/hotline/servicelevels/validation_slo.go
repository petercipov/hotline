package servicelevels

import (
	"hotline/metrics"
	"time"
)

type ValidationStatus string

const (
	ValidationStatusSkipped ValidationStatus = "skipped"
	ValidationStatusSuccess ValidationStatus = "success"
	ValidationStatusFailure ValidationStatus = "failure"
)

type ValidationSLO struct {
	window          *metrics.SlidingWindow[ValidationStatus]
	namespace       string
	tags            map[string]string
	breachThreshold Percentile
}

func NewValidationSLO(
	breachThreshold Percentile,
	windowDuration time.Duration,
	namespace string,
	tags map[string]string,
) *ValidationSLO {
	window := metrics.NewSlidingWindow(func() metrics.Accumulator[ValidationStatus] {
		return metrics.NewTagsHistogram([]ValidationStatus{
			ValidationStatusSkipped,
			ValidationStatusSuccess,
			ValidationStatusFailure,
		})
	}, windowDuration, 1*time.Minute)

	return &ValidationSLO{
		breachThreshold: breachThreshold,
		namespace:       namespace,
		window:          window,
		tags:            tags,
	}
}

func (s *ValidationSLO) AddValidation(now time.Time, status ValidationStatus) {
	s.window.AddValue(now, status)
}

func (s *ValidationSLO) Check(now time.Time) []LevelsCheck {
	activeWindow := s.window.GetActiveWindow(now)
	if activeWindow == nil {
		return nil
	}
	checks := make([]LevelsCheck, 1)
	checks = checks[:0]
	histogram := activeWindow.Accumulator.(*metrics.TagHistogram[ValidationStatus])

	percentage, total := histogram.ComputePercentile(ValidationStatusSkipped)
	if percentage != nil {
		checks = append(checks, LevelsCheck{
			Namespace: s.namespace,
			Timestamp: now,
			Metric: Metric{
				Name:        string(ValidationStatusSkipped),
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
