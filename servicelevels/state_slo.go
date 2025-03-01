package servicelevels

import "time"

type StateSLO struct {
	window    *SlidingWindow[string]
	states    []string
	statesMap map[string]int
}

const unknownStateName = "unknown"

func NewStateSLO(expectedStates []string, windowDuration time.Duration) *StateSLO {
	expectedStates = append(expectedStates, unknownStateName)
	expectedStates = uniqueSlice(expectedStates)

	statesMap := make(map[string]int, len(expectedStates))
	for i, state := range expectedStates {
		statesMap[state] = i
	}

	window := NewSlidingWindow(func() Accumulator[string] {
		return NewTagsHistogram(expectedStates)
	}, windowDuration, 1*time.Minute)
	return &StateSLO{
		window:    window,
		states:    expectedStates,
		statesMap: statesMap,
	}
}

func (s *StateSLO) AddState(now time.Time, state string) {
	_, found := s.statesMap[state]
	if !found {
		state = unknownStateName
	}
	s.window.AddValue(now, state)
}

func (s *StateSLO) Check(now time.Time) []SLOCheck {
	activeWindow := s.window.GetActiveWindow(now)
	if activeWindow == nil {
		return nil
	}

	histogram := activeWindow.Accumulator.(*TagHistogram)
	metrics := make([]SLOCheck, len(s.states))
	metrics = metrics[:0]
	for _, state := range s.states {
		metric := histogram.ComputePercentile(state)
		if metric != nil {
			metrics = append(metrics, SLOCheck{
				MetricName:  state,
				MetricValue: *metric,
			})
		}
	}
	return metrics
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
