package servicelevels_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"hotline/servicelevels"
	"slices"
	"time"
)

var _ = Describe("SlidingWindow", func() {

	s := sutslidingwindow{}

	Context("empty window", func() {
		It("returns NO active window", func() {
			s.forEmptySlidingWindow()
			window := s.getActiveWindow("2025-02-22T12:04:05Z")
			Expect(window).To(BeNil())
		})
	})

	Context("window with single value, time is trimmed to grace period down", func() {
		It("returns NO active window if current time not in grace period", func() {
			s.forEmptySlidingWindow()
			s.addValue(1234, "2025-02-22T12:04:04Z")
			window := s.getActiveWindow("2025-02-22T12:04:00Z")
			Expect(window).To(BeNil())
		})

		It("returns active window if current time falls into grace period", func() {
			s.forEmptySlidingWindow()
			s.addValue(1234, "2025-02-22T12:03:05Z")
			window := s.getActiveWindow("2025-02-22T12:04:00Z")
			Expect(window).NotTo(BeNil())
		})

		It("returns active window containing inserted value", func() {
			s.forEmptySlidingWindow()
			s.addValue(1234, "2025-02-22T12:03:05Z")
			window := s.getActiveWindow("2025-02-22T12:04:00Z")
			Expect(window).NotTo(BeNil())
			Expect(s.windowContains(window, 1234)).To(BeTrue())
		})

	})
})

type sutslidingwindow struct {
	slidingWindow *servicelevels.SlidingWindow
}

func (s *sutslidingwindow) forEmptySlidingWindow() {
	s.slidingWindow = servicelevels.NewSlidingWindow(
		newArrAccumulator,
		1*time.Minute,
		10*time.Second,
	)
}

func (s *sutslidingwindow) getActiveWindow(nowString string) *servicelevels.Window {
	now, parseErr := time.Parse(time.RFC3339, nowString)
	Expect(parseErr).NotTo(HaveOccurred())
	return s.slidingWindow.GetActiveWindow(now)
}

func (s *sutslidingwindow) addValue(latency float64, nowString string) {
	now, parseErr := time.Parse(time.RFC3339, nowString)
	Expect(parseErr).NotTo(HaveOccurred())
	s.slidingWindow.AddValue(now, latency)
}

func (s *sutslidingwindow) windowContains(window *servicelevels.Window, value float64) bool {
	acc := window.Accumulator.(*arrAccumulator)
	return slices.Contains(acc.values, value)
}

type arrAccumulator struct {
	values []float64
}

func newArrAccumulator() servicelevels.Accumulator {
	return &arrAccumulator{}
}

func (a *arrAccumulator) Add(value float64) {
	a.values = append(a.values, value)
}
