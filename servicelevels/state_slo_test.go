package servicelevels_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"hotline/servicelevels"
	"time"
)

var _ = Describe("State SLO", func() {
	sut := stateslosut{}
	Context("no input data", func() {
		It("should return no metrics", func() {
			sut.forEmptySLO()
			metrics := sut.getMetrics()
			Expect(metrics).To(HaveLen(0))
		})
	})

	Context("known input data", func() {
		It("should return metric for single entry", func() {
			sut.forSLO("success", "failure")
			sut.AddState("success")
			metrics := sut.getMetrics()
			Expect(metrics).To(HaveLen(1))
			Expect(metrics).To(Equal(map[string]float64{
				"success": 100,
			}))
		})

		It("should return metric for multiple entry", func() {
			sut.forSLO("success", "failure")
			sut.AddState("success")
			sut.AddState("success")
			sut.AddState("success")
			sut.AddState("failure")
			metrics := sut.getMetrics()
			Expect(metrics).To(HaveLen(2))
			Expect(metrics).To(Equal(map[string]float64{
				"success": 75,
				"failure": 25,
			}))
		})
	})

	Context("unknown input data", func() {
		It("should return unknown metric for unknown state", func() {
			sut.forSLO("success", "failure")
			sut.AddState("success")
			sut.AddState("success")
			sut.AddState("success")
			sut.AddState("abcd")
			metrics := sut.getMetrics()
			Expect(metrics).To(HaveLen(2))
			Expect(metrics).To(Equal(map[string]float64{
				"success": 75,
				"unknown": 25,
			}))
		})

		It("should deduplicate unknown metric for unknown state", func() {
			sut.forSLO("unknown", "success", "unknown", "success", "unknown", "success")
			sut.AddState("success")
			sut.AddState("success")
			sut.AddState("success")
			sut.AddState("abcd")
			metrics := sut.getMetrics()
			Expect(metrics).To(HaveLen(2))
			Expect(metrics).To(Equal(map[string]float64{
				"success": 75,
				"unknown": 25,
			}))
		})
	})

})

type stateslosut struct {
	slo *servicelevels.StateSLO
}

func (s *stateslosut) forEmptySLO() {
	s.forSLO()
}

func (s *stateslosut) getMetrics() map[string]float64 {
	now := parseTime("2025-02-22T12:04:55Z")
	states := s.slo.ListStates()
	metrics := s.slo.GetMetrics(now)
	result := map[string]float64{}
	for i, state := range states {
		if metrics[i] != 0 {
			result[state] = metrics[i]
		}
	}
	return result
}

func (s *stateslosut) forSLO(states ...string) {
	s.slo = servicelevels.NewStateSLO(states, 1*time.Hour)
}

func (s *stateslosut) AddState(state string) {
	now := parseTime("2025-02-22T12:04:55Z")
	s.slo.AddState(now, state)
}
