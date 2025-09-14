package servicelevels

import "time"

type HttpStatusSLO struct {
	stateSLO  *StateSLO
	expected  map[string]bool
	breakdown *HttpStateRangeBreakdown
}

func NewHttpStatusSLO(
	expectedHttpState []string,
	breachThreshold Percentile,
	windowDuration time.Duration,
	tags map[string]string,
) *HttpStatusSLO {

	breakdown := NewHttpStateRangeBreakdown()
	expected := make(map[string]bool)

	for _, status := range expectedHttpState {
		expected[status] = true
	}
	stateSLO := NewStateSLO(
		expectedHttpState,
		breakdown.GetRanges(),
		breachThreshold,
		windowDuration,
		"http_route_status",
		tags,
	)

	return &HttpStatusSLO{
		stateSLO:  stateSLO,
		expected:  expected,
		breakdown: breakdown,
	}
}

func (s *HttpStatusSLO) AddHttpState(now time.Time, state string) {
	_, isExpected := s.expected[state]
	if isExpected {
		s.stateSLO.AddState(now, state)
		return
	}

	httpRange := s.breakdown.GetRange(state)
	if httpRange != nil {
		s.stateSLO.AddState(now, *httpRange)
		return
	}

	s.stateSLO.AddState(now, httpRangeUnknown)
}

func (s *HttpStatusSLO) Check(now time.Time) []SLOCheck {
	return s.stateSLO.Check(now)
}
