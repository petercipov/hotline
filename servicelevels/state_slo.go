package servicelevels

import (
	"math"
	"strings"
	"time"
)

type StateSLO struct {
	window            *SlidingWindow[string]
	expectedStates    []string
	expectedStatesMap map[string]int
	breachThreshold   float64
}

const unexpectedStateName = "unexpected"
const expectedStateName = "expected"

func NewStateSLO(expectedStates []string, breachThreshold Percent, windowDuration time.Duration) *StateSLO {
	expectedStates = uniqueSlice(filterOutUnknownTag(expectedStates))

	statesMap := make(map[string]int, len(expectedStates))
	for i, state := range expectedStates {
		statesMap[state] = i
	}

	tags := append([]string{}, expectedStates...)
	tags = append(tags, unexpectedStateName)
	window := NewSlidingWindow(func() Accumulator[string] {
		return NewTagsHistogram(tags)
	}, windowDuration, 1*time.Minute)
	return &StateSLO{
		window:            window,
		expectedStates:    expectedStates,
		expectedStatesMap: statesMap,
		breachThreshold:   roundTo(float64(breachThreshold), 5),
	}
}

func filterOutUnknownTag(states []string) []string {
	filteredStates := make([]string, len(states))
	filteredStates = filteredStates[:0]
	for _, state := range states {
		if !strings.EqualFold(state, unexpectedStateName) {
			filteredStates = append(filteredStates, state)
		}
	}
	return filteredStates
}

func (s *StateSLO) AddState(now time.Time, state string) {
	_, found := s.expectedStatesMap[state]
	if !found {
		state = unexpectedStateName
	}
	s.window.AddValue(now, state)
}

func (s *StateSLO) Check(now time.Time) []SLOCheck {
	activeWindow := s.window.GetActiveWindow(now)
	if activeWindow == nil {
		return nil
	}
	histogram := activeWindow.Accumulator.(*TagHistogram)

	expectedBreach, expectedMetric, expectedBreakdown := s.checkExpectedBreach(histogram)
	unexpectedBreach, unexpectedMetric := s.checkUnexpectedBreach(histogram)

	checks := make([]SLOCheck, 2)
	checks = checks[:0]
	if len(expectedBreakdown) > 0 {
		checks = append(checks, SLOCheck{
			Metric: Metric{
				Name:  expectedStateName,
				Value: expectedMetric,
			},
			Breakdown: expectedBreakdown,
			Breach:    expectedBreach,
		})
	}

	if unexpectedBreach != nil {
		checks = append(checks, SLOCheck{
			Metric: Metric{
				Name:  unexpectedStateName,
				Value: unexpectedMetric,
			},
			Breach: unexpectedBreach,
		})
	}

	return checks

}

func (s *StateSLO) checkUnexpectedBreach(histogram *TagHistogram) (*SLOBreach, float64) {
	unexpectedMetric := histogram.ComputePercentile(unexpectedStateName)

	var breach *SLOBreach
	var value float64 = 0
	if unexpectedMetric != nil {
		breach = &SLOBreach{
			ThresholdValue: roundTo(100.0-s.breachThreshold, 5),
			ThresholdUnit:  "%",
			Operation:      OperationL,
			WindowDuration: s.window.Size,
		}
		value = *unexpectedMetric
	}

	return breach, value
}

func (s *StateSLO) checkExpectedBreach(histogram *TagHistogram) (*SLOBreach, float64, []Metric) {
	breakDown := make([]Metric, len(s.expectedStates))
	breakDown = breakDown[:0]
	expectedSum := float64(0)
	for _, state := range s.expectedStates {
		metric := histogram.ComputePercentile(state)
		if metric != nil {
			breakDown = append(breakDown, Metric{
				Name:  state,
				Value: *metric,
			})
			expectedSum += *metric
		}
	}
	var breach *SLOBreach
	sloHolds := expectedSum >= s.breachThreshold
	if !sloHolds {
		breach = &SLOBreach{
			ThresholdValue: s.breachThreshold,
			ThresholdUnit:  "%",
			Operation:      OperationGE,
			WindowDuration: s.window.Size,
		}
	}
	return breach, expectedSum, breakDown
}

func uniqueSlice(values []string) []string {
	uniqueValues := make(map[string]bool, len(values))
	newArr := make([]string, len(values))
	newArr = newArr[:0]
	for _, elem := range values {
		if !uniqueValues[elem] {
			newArr = append(newArr, elem)
			uniqueValues[elem] = true
		}
	}
	return newArr
}

func roundTo(value float64, decimals uint32) float64 {
	return math.Round(value*math.Pow(10, float64(decimals))) / math.Pow(10, float64(decimals))
}
