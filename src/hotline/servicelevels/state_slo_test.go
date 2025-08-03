package servicelevels_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"hotline/clock"
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
			sut.forSLO("state1", "state2")
			sut.AddState("state1")
			metrics := sut.getMetrics()
			Expect(metrics).To(HaveLen(1))
			Expect(metrics[0]).To(Equal(servicelevels.SLOCheck{
				Namespace: "test-namespace",
				Metric: servicelevels.Metric{
					Name:        "expected",
					Value:       100,
					Unit:        "%",
					EventsCount: 1,
				},
				Breakdown: []servicelevels.Metric{
					{
						Name:        "state1",
						Value:       100,
						Unit:        "%",
						EventsCount: 1,
					},
				},
			}))
		})

		It("should return metric for multiple entry", func() {
			sut.forSLO("state1", "state2")
			sut.AddState("state1")
			sut.AddState("state1")
			sut.AddState("state1")
			sut.AddState("state2")
			metrics := sut.getMetrics()
			Expect(metrics).To(HaveLen(1))
			Expect(metrics[0]).To(Equal(servicelevels.SLOCheck{
				Namespace: "test-namespace",
				Metric: servicelevels.Metric{
					Name:        "expected",
					Value:       100,
					Unit:        "%",
					EventsCount: 4,
				},
				Breakdown: []servicelevels.Metric{
					{
						Name:        "state1",
						Value:       75,
						Unit:        "%",
						EventsCount: 3,
					}, {
						Name:        "state2",
						Value:       25,
						Unit:        "%",
						EventsCount: 1,
					},
				},
			}))
		})
	})

	Context("percentile of expected requests states in window of 1h,", func() {
		It("has not been Breached if more than 99.99 %", func() {
			sut.forSLO("20x", "30x")
			sut.AddState("20x")
			sut.AddState("20x")
			sut.AddState("20x")
			sut.AddState("30x")

			metrics := sut.getMetrics()
			Expect(metrics).To(HaveLen(1))
			Expect(metrics[0]).To(Equal(servicelevels.SLOCheck{
				Namespace: "test-namespace",
				Metric: servicelevels.Metric{
					Name:        "expected",
					Value:       100,
					Unit:        "%",
					EventsCount: 4,
				},
				Breakdown: []servicelevels.Metric{
					{
						Name:        "20x",
						Value:       75,
						Unit:        "%",
						EventsCount: 3,
					}, {
						Name:        "30x",
						Value:       25,
						Unit:        "%",
						EventsCount: 1,
					},
				},
			}))
		})

		It("has been Breached if less than 99.99 %", func() {
			sut.forSLO("20x", "30x")
			sut.AddState("20x")
			sut.AddState("20x")
			sut.AddState("20x")
			sut.AddState("400x")

			metrics := sut.getMetrics()
			Expect(metrics).To(HaveLen(2))
			Expect(metrics[0]).To(Equal(servicelevels.SLOCheck{
				Namespace: "test-namespace",
				Metric: servicelevels.Metric{
					Name:        "expected",
					Value:       75,
					Unit:        "%",
					EventsCount: 3,
				},
				Breakdown: []servicelevels.Metric{
					{
						Name:        "20x",
						Value:       75,
						Unit:        "%",
						EventsCount: 3,
					},
				},
				Breach: &servicelevels.SLOBreach{
					ThresholdValue: 99.99,
					ThresholdUnit:  "%",
					Operation:      servicelevels.OperationGE,
					WindowDuration: 1 * time.Hour,
				},
			}))
			Expect(metrics[1]).To(Equal(servicelevels.SLOCheck{
				Namespace: "test-namespace",
				Metric: servicelevels.Metric{
					Name:        "unexpected",
					Value:       25,
					Unit:        "%",
					EventsCount: 1,
				},
				Breakdown: []servicelevels.Metric{
					{
						Name:        "unexpected",
						Value:       25,
						Unit:        "%",
						EventsCount: 1,
					},
				},
				Breach: &servicelevels.SLOBreach{
					ThresholdValue: 0.01,
					ThresholdUnit:  "%",
					Operation:      servicelevels.OperationL,
					WindowDuration: 1 * time.Hour,
				},
			}))
		})

		It("has been Breached if only unexpected", func() {
			sut.forSLO("20x", "30x")
			sut.AddState("400x")
			sut.AddState("400x")
			sut.AddState("400x")
			sut.AddState("400x")

			metrics := sut.getMetrics()
			Expect(metrics).To(HaveLen(1))
			Expect(metrics[0]).To(Equal(servicelevels.SLOCheck{
				Namespace: "test-namespace",
				Metric: servicelevels.Metric{
					Name:        "unexpected",
					Value:       100,
					Unit:        "%",
					EventsCount: 4,
				},
				Breakdown: []servicelevels.Metric{
					{
						Name:        "unexpected",
						Value:       100,
						Unit:        "%",
						EventsCount: 4,
					},
				},
				Breach: &servicelevels.SLOBreach{
					ThresholdValue: 0.01,
					ThresholdUnit:  "%",
					Operation:      servicelevels.OperationL,
					WindowDuration: 1 * time.Hour,
				},
			}))
		})
	})

	Context("unexpected input data", func() {
		It("should return unexpected metric for unexpected state", func() {
			sut.forSLO("state1", "state2")
			sut.AddState("state1")
			sut.AddState("state1")
			sut.AddState("state1")
			sut.AddState("abcd")
			metrics := sut.getMetrics()
			Expect(metrics).To(HaveLen(2))
			Expect(metrics[0]).To(Equal(servicelevels.SLOCheck{
				Namespace: "test-namespace",
				Metric: servicelevels.Metric{
					Name:        "expected",
					Value:       75,
					Unit:        "%",
					EventsCount: 3,
				},
				Breakdown: []servicelevels.Metric{
					{
						Name:        "state1",
						Value:       75,
						Unit:        "%",
						EventsCount: 3,
					},
				},
				Breach: &servicelevels.SLOBreach{
					ThresholdValue: 99.99,
					ThresholdUnit:  "%",
					Operation:      servicelevels.OperationGE,
					WindowDuration: 1 * time.Hour,
				},
			}))
			Expect(metrics[1]).To(Equal(servicelevels.SLOCheck{
				Namespace: "test-namespace",
				Metric: servicelevels.Metric{
					Name:        "unexpected",
					Value:       25,
					Unit:        "%",
					EventsCount: 1,
				},
				Breakdown: []servicelevels.Metric{
					{
						Name:        "unexpected",
						Value:       25,
						Unit:        "%",
						EventsCount: 1,
					},
				},
				Breach: &servicelevels.SLOBreach{
					ThresholdValue: 0.01,
					ThresholdUnit:  "%",
					Operation:      servicelevels.OperationL,
					WindowDuration: 1 * time.Hour,
				},
			}))
		})

		It("should deduplicate unexpected metric for unexpected state", func() {
			sut.forSLO("unexpected", "state1", "unexpected", "state1", "unexpected", "state1")
			sut.AddState("state1")
			sut.AddState("state1")
			sut.AddState("state1")
			sut.AddState("abcd")
			metrics := sut.getMetrics()
			Expect(metrics).To(HaveLen(2))
			Expect(metrics[0]).To(Equal(servicelevels.SLOCheck{
				Namespace: "test-namespace",
				Metric: servicelevels.Metric{
					Name:        "expected",
					Value:       75,
					Unit:        "%",
					EventsCount: 3,
				},
				Breakdown: []servicelevels.Metric{
					{
						Name:        "state1",
						Value:       75,
						Unit:        "%",
						EventsCount: 3,
					},
				},
				Breach: &servicelevels.SLOBreach{
					ThresholdValue: 99.99,
					ThresholdUnit:  "%",
					Operation:      servicelevels.OperationGE,
					WindowDuration: 1 * time.Hour,
				},
			}))
			Expect(metrics[1]).To(Equal(servicelevels.SLOCheck{
				Namespace: "test-namespace",
				Metric: servicelevels.Metric{
					Name:        "unexpected",
					Value:       25,
					Unit:        "%",
					EventsCount: 1,
				},
				Breakdown: []servicelevels.Metric{
					{
						Name:        "unexpected",
						Value:       25,
						Unit:        "%",
						EventsCount: 1,
					},
				},
				Breach: &servicelevels.SLOBreach{
					ThresholdValue: 0.01,
					ThresholdUnit:  "%",
					Operation:      servicelevels.OperationL,
					WindowDuration: 1 * time.Hour,
				},
			}))
		})

		It("should trace unexpected state if defined and show in breakdown", func() {
			sut.forSLOTRackingUnexpected([]string{"state1", "state2"}, []string{"timeout"})
			sut.AddState("state1")
			sut.AddState("state1")
			sut.AddState("state1")
			sut.AddState("timeout")
			metrics := sut.getMetrics()
			Expect(metrics).To(HaveLen(2))
			Expect(metrics[0]).To(Equal(servicelevels.SLOCheck{
				Namespace: "test-namespace",
				Metric: servicelevels.Metric{
					Name:        "expected",
					Value:       75,
					Unit:        "%",
					EventsCount: 3,
				},
				Breakdown: []servicelevels.Metric{
					{
						Name:        "state1",
						Value:       75,
						Unit:        "%",
						EventsCount: 3,
					},
				},
				Breach: &servicelevels.SLOBreach{
					ThresholdValue: 99.99,
					ThresholdUnit:  "%",
					Operation:      servicelevels.OperationGE,
					WindowDuration: 1 * time.Hour,
				},
			}))
			Expect(metrics[1]).To(Equal(servicelevels.SLOCheck{
				Namespace: "test-namespace",
				Metric: servicelevels.Metric{
					Name:        "unexpected",
					Value:       25,
					Unit:        "%",
					EventsCount: 1,
				},
				Breakdown: []servicelevels.Metric{
					{
						Name:        "timeout",
						Value:       25,
						Unit:        "%",
						EventsCount: 1,
					},
				},
				Breach: &servicelevels.SLOBreach{
					ThresholdValue: 0.01,
					ThresholdUnit:  "%",
					Operation:      servicelevels.OperationL,
					WindowDuration: 1 * time.Hour,
				},
			}))
		})

		It("should trace unexpected breakdown when added unknown", func() {
			sut.forSLOTRackingUnexpected([]string{"state1", "state2"}, []string{"timeout"})
			sut.AddState("state1")
			sut.AddState("state1")
			sut.AddState("state1")
			sut.AddState("timeout")
			sut.AddState("unknnown_unknown")
			metrics := sut.getMetrics()
			Expect(metrics).To(HaveLen(2))
			Expect(metrics[0]).To(Equal(servicelevels.SLOCheck{
				Namespace: "test-namespace",
				Metric: servicelevels.Metric{
					Name:        "expected",
					Value:       60,
					Unit:        "%",
					EventsCount: 3,
				},
				Breakdown: []servicelevels.Metric{
					{
						Name:        "state1",
						Value:       60,
						Unit:        "%",
						EventsCount: 3,
					},
				},
				Breach: &servicelevels.SLOBreach{
					ThresholdValue: 99.99,
					ThresholdUnit:  "%",
					Operation:      servicelevels.OperationGE,
					WindowDuration: 1 * time.Hour,
				},
			}))
			Expect(metrics[1]).To(Equal(servicelevels.SLOCheck{
				Namespace: "test-namespace",
				Metric: servicelevels.Metric{
					Name:        "unexpected",
					Value:       40,
					Unit:        "%",
					EventsCount: 2,
				},
				Breakdown: []servicelevels.Metric{
					{
						Name:        "timeout",
						Value:       20,
						Unit:        "%",
						EventsCount: 1,
					},
					{
						Name:        "unexpected",
						Value:       20,
						Unit:        "%",
						EventsCount: 1,
					},
				},
				Breach: &servicelevels.SLOBreach{
					ThresholdValue: 0.01,
					ThresholdUnit:  "%",
					Operation:      servicelevels.OperationL,
					WindowDuration: 1 * time.Hour,
				},
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

func (s *stateslosut) getMetrics() []servicelevels.SLOCheck {
	now := clock.ParseTime("2025-02-22T12:04:55Z")
	return s.slo.Check(now)
}

func (s *stateslosut) forSLO(expectedStates ...string) {
	s.forSLOTRackingUnexpected(expectedStates, nil)
}

func (s *stateslosut) forSLOTRackingUnexpected(expectedStates []string, unexpectedStates []string) {
	p, failure := servicelevels.ParsePercentile(99.99)
	Expect(failure).To(BeNil())

	s.slo = servicelevels.NewStateSLO(expectedStates, unexpectedStates, p, 1*time.Hour, "test-namespace", nil)
}

func (s *stateslosut) AddState(state string) {
	now := clock.ParseTime("2025-02-22T12:04:55Z")
	s.slo.AddState(now, state)
}
