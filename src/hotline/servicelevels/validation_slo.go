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
	createAt        time.Time
}

func NewValidationSLO(
	breachThreshold Percentile,
	windowDuration time.Duration,
	namespace string,
	tags map[string]string,
	now time.Time,
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
		createAt:        now,
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
	uptime := now.Sub(s.createAt)
	histogram := activeWindow.Accumulator.(*metrics.TagHistogram[ValidationStatus])

	skippedPercentage, totalSkipped := histogram.ComputePercentile(ValidationStatusSkipped)
	successPercentage, totalSuccess := histogram.ComputePercentile(ValidationStatusSuccess)
	failurePercentage, totalFailure := histogram.ComputePercentile(ValidationStatusFailure)

	if skippedPercentage == nil && successPercentage == nil && failurePercentage == nil {
		return nil
	}

	total := histogram.Total()

	var breach *SLOBreach
	isBreached := optFloat(successPercentage) < s.breachThreshold.AsPercent()
	if isBreached {
		breach = &SLOBreach{
			ThresholdValue: s.breachThreshold.AsPercent(),
			ThresholdUnit:  "%",
			Operation:      OperationL,
			WindowDuration: s.window.Size,
			Uptime:         uptime,
		}
	}

	return []LevelsCheck{
		{
			Namespace: s.namespace,
			Metric: Metric{
				Name:        "valid_requests",
				Value:       optFloat(successPercentage),
				Unit:        "%",
				EventsCount: total,
			},
			Tags: s.tags,
			Breakdown: []Metric{
				{
					Name:        "skipped",
					Value:       optFloat(skippedPercentage),
					Unit:        "%",
					EventsCount: totalSkipped,
				},
				{
					Name:        "valid",
					Value:       optFloat(successPercentage),
					Unit:        "%",
					EventsCount: totalSuccess,
				},
				{
					Name:        "invalid",
					Value:       optFloat(failurePercentage),
					Unit:        "%",
					EventsCount: totalFailure,
				},
			},
			Breach:    breach,
			Timestamp: now,
		},
	}
}

func optFloat(val *float64) float64 {
	if val == nil {
		return 0
	}
	return *val
}
