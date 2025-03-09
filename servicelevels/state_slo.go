package servicelevels

import (
	"math"
	"strings"
	"time"
)

type StateSLO struct {
	window                    *SlidingWindow[string]
	expectedStates            []string
	expectedStatesMap         map[string]int
	unexpectedStates          []string
	unexpectedStatesMap       map[string]int
	breachThreshold           float64
	unexpectedBreachThreshold float64
	tags                      map[string]string
}

const unexpectedStateName = "unexpected"
const expectedStateName = "expected"

func NewStateSLO(
	expectedStates []string,
	unexpectedStates []string,
	breachThreshold Percent,
	windowDuration time.Duration,
	tags map[string]string) *StateSLO {
	expectedStates = uniqueSlice(filterOutUnknownTag(expectedStates))
	unexpectedStates = uniqueSlice(filterOutUnknownTag(unexpectedStates))
	unexpectedStates = append(unexpectedStates, unexpectedStateName)

	statesMap := make(map[string]int, len(expectedStates))
	for i, state := range expectedStates {
		statesMap[state] = i
	}
	unexpectedStatesMap := make(map[string]int, len(unexpectedStates))
	for i, state := range unexpectedStates {
		unexpectedStatesMap[state] = i
	}

	stateNames := append([]string{}, expectedStates...)
	stateNames = append(stateNames, unexpectedStates...)
	window := NewSlidingWindow(func() Accumulator[string] {
		return NewTagsHistogram(stateNames)
	}, windowDuration, 1*time.Minute)

	expectedBreachThreshold := roundTo(breachThreshold.Value(), 5)
	unexpectedBreachThreshold := roundTo(100.0-expectedBreachThreshold, 5)
	return &StateSLO{
		window:                    window,
		expectedStates:            expectedStates,
		expectedStatesMap:         statesMap,
		unexpectedStates:          unexpectedStates,
		unexpectedStatesMap:       unexpectedStatesMap,
		breachThreshold:           expectedBreachThreshold,
		unexpectedBreachThreshold: unexpectedBreachThreshold,
		tags:                      tags,
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
		_, found = s.unexpectedStatesMap[state]
		if !found {
			state = unexpectedStateName
		}
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
	unexpectedBreach, unexpectedMetric, unexpectedBreakdown := s.checkUnexpectedBreach(histogram)

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
			Tags:      s.tags,
		})
	}

	if unexpectedBreach != nil {
		checks = append(checks, SLOCheck{
			Metric: Metric{
				Name:  unexpectedStateName,
				Value: unexpectedMetric,
			},
			Breakdown: unexpectedBreakdown,
			Breach:    unexpectedBreach,
			Tags:      s.tags,
		})
	}

	return checks

}

func (s *StateSLO) checkUnexpectedBreach(histogram *TagHistogram) (*SLOBreach, float64, []Metric) {
	breakDown := make([]Metric, len(s.unexpectedStatesMap))
	breakDown = breakDown[:0]
	unexpectedSum := float64(0)
	for _, state := range s.unexpectedStates {
		metric := histogram.ComputePercentile(state)
		if metric != nil {
			breakDown = append(breakDown, Metric{
				Name:  state,
				Value: *metric,
			})
			unexpectedSum += *metric
		}
	}

	var breach *SLOBreach
	if unexpectedSum > s.unexpectedBreachThreshold {
		breach = &SLOBreach{
			ThresholdValue: s.unexpectedBreachThreshold,
			ThresholdUnit:  "%",
			Operation:      OperationL,
			WindowDuration: s.window.Size,
		}
	}

	return breach, unexpectedSum, breakDown
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
