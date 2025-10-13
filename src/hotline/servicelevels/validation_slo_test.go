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
})

type validationSloSut struct {
	slo   *servicelevels.ValidationSLO
	clock *clock.ManualClock
}

func (s *validationSloSut) forValidationSLO() {
	s.slo = servicelevels.NewValidationSLO(
		1*time.Hour,
		"http_route_validation",
		map[string]string{
			"test": "tag",
		})
	s.clock = clock.NewManualClock(
		clock.ParseTime("2025-02-22T12:02:10Z"),
		500*time.Microsecond,
	)
}

func (s *validationSloSut) Check() []servicelevels.LevelsCheck {
	return s.slo.Check(s.clock.Now())
}
