package servicelevels_test

import (
	"hotline/clock"
	"hotline/servicelevels"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Validation SLO", func() {
	sut := validationSloSut{}
	It("empty slo will return no checks", func() {
		sut.forValidationSLO()
		levels := sut.Check()
		Expect(levels).To(BeEmpty())
	})

	It("computes checks for skipped request validation", func() {
		sut.forValidationSLO()
		for range 10 {
			sut.applySkippedRequestValidation()
		}
		levels := sut.Check()
		Expect(levels).To(HaveLen(1))
		Expect(levels[0]).To(Equal(servicelevels.LevelsCheck{
			Namespace: "http_route_validation",
			Metric: servicelevels.Metric{
				Name:        "valid_requests",
				Value:       0,
				Unit:        "%",
				EventsCount: 10,
			},
			Tags: map[string]string{
				"test": "tag",
			},
			Breakdown: []servicelevels.Metric{
				{
					Name:        "skipped",
					Value:       100,
					Unit:        "%",
					EventsCount: 10,
				},
				{
					Name:        "valid",
					Value:       0,
					Unit:        "%",
					EventsCount: 0,
				},
				{
					Name:        "invalid",
					Value:       0,
					Unit:        "%",
					EventsCount: 0,
				},
			},
			Breach: &servicelevels.SLOBreach{
				ThresholdValue: 99.9,
				ThresholdUnit:  "%",
				Operation:      servicelevels.OperationL,
				WindowDuration: 1 * time.Hour,
				Uptime:         1*time.Minute + 5*time.Millisecond + 500*time.Microsecond,
			},
			Timestamp: clock.ParseTime("2025-02-22T12:03:10.0055Z"),
		}))
	})

	It("computes checks for successful request validation", func() {
		sut.forValidationSLO()
		for range 10 {
			sut.applySuccessRequestValidation()
		}
		levels := sut.Check()
		Expect(levels).To(HaveLen(1))
		Expect(levels[0]).To(Equal(servicelevels.LevelsCheck{
			Namespace: "http_route_validation",
			Metric: servicelevels.Metric{
				Name:        "valid_requests",
				Value:       100,
				Unit:        "%",
				EventsCount: 10,
			},
			Tags: map[string]string{
				"test": "tag",
			},
			Breakdown: []servicelevels.Metric{
				{
					Name:        "skipped",
					Value:       0,
					Unit:        "%",
					EventsCount: 0,
				},
				{
					Name:        "valid",
					Value:       100,
					Unit:        "%",
					EventsCount: 10,
				},
				{
					Name:        "invalid",
					Value:       0,
					Unit:        "%",
					EventsCount: 0,
				},
			},
			Breach:    nil,
			Timestamp: clock.ParseTime("2025-02-22T12:03:10.0055Z"),
		}))
	})

	It("computes checks for request validation failure", func() {
		sut.forValidationSLO()
		for range 10 {
			sut.applyFailedRequestValidation()
		}
		levels := sut.Check()
		Expect(levels).To(HaveLen(1))
		Expect(levels[0]).To(Equal(servicelevels.LevelsCheck{
			Namespace: "http_route_validation",
			Metric: servicelevels.Metric{
				Name:        "valid_requests",
				Value:       0,
				Unit:        "%",
				EventsCount: 10,
			},
			Tags: map[string]string{
				"test": "tag",
			},
			Breakdown: []servicelevels.Metric{
				{
					Name:        "skipped",
					Value:       0,
					Unit:        "%",
					EventsCount: 0,
				},
				{
					Name:        "valid",
					Value:       0,
					Unit:        "%",
					EventsCount: 0,
				},
				{
					Name:        "invalid",
					Value:       100,
					Unit:        "%",
					EventsCount: 10,
				},
			},
			Breach: &servicelevels.SLOBreach{
				ThresholdValue: 99.9,
				ThresholdUnit:  "%",
				Operation:      servicelevels.OperationL,
				WindowDuration: 1 * time.Hour,
				Uptime:         1*time.Minute + 5*time.Millisecond + 500*time.Microsecond,
			},
			Timestamp: clock.ParseTime("2025-02-22T12:03:10.0055Z"),
		}))
	})
})

type validationSloSut struct {
	slo   *servicelevels.ValidationSLO
	clock *clock.ManualClock
}

func (s *validationSloSut) forValidationSLO() {
	s.clock = clock.NewDefaultManualClock()
	s.slo = servicelevels.NewValidationSLO(
		servicelevels.P999,
		1*time.Hour,
		"http_route_validation",
		map[string]string{
			"test": "tag",
		},
		s.clock.Now(),
	)
	s.clock.Advance(1 * time.Minute)
}

func (s *validationSloSut) Check() []servicelevels.LevelsCheck {
	return s.slo.Check(s.clock.Now())
}

func (s *validationSloSut) applySuccessRequestValidation() {
	s.slo.AddValidation(s.clock.Now(), servicelevels.ValidationStatusSuccess)
}

func (s *validationSloSut) applySkippedRequestValidation() {
	s.slo.AddValidation(s.clock.Now(), servicelevels.ValidationStatusSkipped)
}

func (s *validationSloSut) applyFailedRequestValidation() {
	s.slo.AddValidation(s.clock.Now(), servicelevels.ValidationStatusFailure)
}
