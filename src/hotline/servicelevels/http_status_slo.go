package servicelevels

import (
	"hotline/metrics"
	"slices"
	"strings"
	"time"
)

const unexpectedStateName = "unexpected"
const expectedStateName = "expected"

type HttpStatusSLO struct {
	window       *metrics.SlidingWindow[string, *metrics.TagHistogram[string]]
	breakdown    *HttpStateRangeBreakdown
	expectations *httpStatusExpectations

	namespace string
	tags      map[string]string
	createdAt time.Time
}

func NewHttpStatusSLO(
	expectedHttpState []string,
	breachThreshold Percentile,
	windowDuration time.Duration,
	tags map[string]string,
	createdAt time.Time,
) *HttpStatusSLO {
	breakdown := NewHttpStateRangeBreakdown()
	expectations := buildExpectations(
		expectedHttpState,
		breakdown.GetRanges(),
		breachThreshold,
	)
	allStatuses := expectations.AllStatuses()
	window := metrics.NewSlidingWindow(func() *metrics.TagHistogram[string] {
		return metrics.NewTagsHistogram(allStatuses)
	}, windowDuration, 1*time.Minute)

	return &HttpStatusSLO{
		window:       window,
		expectations: expectations,
		breakdown:    breakdown,
		namespace:    "http_route_status",
		tags:         tags,
		createdAt:    createdAt,
	}
}

func (s *HttpStatusSLO) AddHttpState(now time.Time, state string) {
	foundState := s.expectations.GetState(state)
	if foundState != "" {
		s.window.AddValue(now, foundState)
		return
	}

	httpRange := s.breakdown.ConvertStateToRange(state)
	if httpRange != nil {
		s.window.AddValue(now, *httpRange)
		return
	}

	s.window.AddValue(now, httpRangeUnknown)
}

func (s *HttpStatusSLO) Check(now time.Time) []LevelsCheck {
	activeWindow := s.window.GetActiveWindow(now)
	if activeWindow == nil {
		return nil
	}
	histogram := activeWindow.Accumulator
	uptime := now.Sub(s.createdAt)

	expectedMetric, expectedEventsCount, expectedBreakdown := s.expectations.checkExpectedBreach(histogram, s.window.Size)
	unexpectedMetric, unexpectedEventsCount, unexpectedBreakdown := s.expectations.checkUnexpectedBreach(histogram, s.window.Size)

	checks := make([]LevelsCheck, 2)
	checks = checks[:0]

	if len(expectedBreakdown) > 0 {
		checks = append(checks, LevelsCheck{
			Namespace: s.namespace,
			Timestamp: now,
			Uptime:    uptime,
			Metric: Metric{
				Name:        expectedStateName,
				Value:       expectedMetric,
				Unit:        "%",
				EventsCount: expectedEventsCount,
			},
			Breakdown: expectedBreakdown,
			Tags:      s.tags,
		})
	}

	if len(unexpectedBreakdown) > 0 {
		checks = append(checks, LevelsCheck{
			Namespace: s.namespace,
			Timestamp: now,
			Uptime:    uptime,
			Metric: Metric{
				Name:        unexpectedStateName,
				Value:       unexpectedMetric,
				Unit:        "%",
				EventsCount: unexpectedEventsCount,
			},
			Breakdown: unexpectedBreakdown,
			Tags:      s.tags,
		})
	}

	return checks

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

type httpStatusExpectations struct {
	expected      []string
	unexpected    []string
	expectedMap   map[string]int
	unexpectedMap map[string]int

	expectedThreshold   float64
	unexpectedThreshold float64
}

func (e *httpStatusExpectations) AllStatuses() []string {
	return slices.Concat(e.expected, e.unexpected)
}

func (e *httpStatusExpectations) GetState(state string) string {
	_, found := e.expectedMap[state]
	if !found {
		_, found = e.unexpectedMap[state]
		if !found {
			state = ""
		}
	}
	return state
}

func (e *httpStatusExpectations) checkExpectedBreach(histogram *metrics.TagHistogram[string], windowSize time.Duration) (float64, int64, []Metric) {
	breakDown := make([]Metric, len(e.expected))
	breakDown = breakDown[:0]
	expectedSum := float64(0)
	eventsSum := int64(0)

	for _, state := range e.expected {
		metric, count := histogram.ComputePercentile(state)
		eventsSum += count
		if metric != nil {
			breakDown = append(breakDown, Metric{
				Name:        state,
				Value:       *metric,
				Unit:        "%",
				EventsCount: count,
			})
			expectedSum += *metric
		}
	}
	return expectedSum, eventsSum, breakDown
}

func (e *httpStatusExpectations) checkUnexpectedBreach(histogram *metrics.TagHistogram[string], windowSize time.Duration) (float64, int64, []Metric) {
	breakDown := make([]Metric, len(e.unexpected))
	breakDown = breakDown[:0]
	unexpectedSum := float64(0)
	eventsSum := int64(0)

	for _, state := range e.unexpected {
		metric, count := histogram.ComputePercentile(state)
		eventsSum += count
		if metric != nil {
			breakDown = append(breakDown, Metric{
				Name:        state,
				Value:       *metric,
				Unit:        "%",
				EventsCount: count,
			})
			unexpectedSum += *metric
		}
	}

	return unexpectedSum, eventsSum, breakDown
}

func buildExpectations(expectedHttpState []string, unexpectedHttpState []string, breachThreshold Percentile) *httpStatusExpectations {
	expected := uniqueSlice(filterOutUnknownTag(expectedHttpState))
	unexpected := uniqueSlice(filterOutUnknownTag(unexpectedHttpState))
	unexpected = append(unexpected, unexpectedStateName, "timeout")

	expectedMap := make(map[string]int, len(expected))
	for i, state := range expected {
		expectedMap[state] = i
	}
	unexpectedMap := make(map[string]int, len(unexpected))
	for i, state := range unexpected {
		unexpectedMap[state] = i
	}

	expectedThreshold := breachThreshold.RoundTo(5)
	invertedThreshold := expectedThreshold.Invert()
	unexpectedThreshold := invertedThreshold.RoundTo(5)

	return &httpStatusExpectations{
		expected:      expected,
		unexpected:    unexpected,
		expectedMap:   expectedMap,
		unexpectedMap: unexpectedMap,

		expectedThreshold:   expectedThreshold.AsPercent(),
		unexpectedThreshold: unexpectedThreshold.AsPercent(),
	}
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
