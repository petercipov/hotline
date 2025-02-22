package servicelevels_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"hotline/servicelevels"
	"time"
)

var _ = Describe("SlidingWindow", func() {

	s := sutslidingwindow{}

	Context("empty window", func() {
		It("returns empty window", func() {
			s.forEmptySlidingWindow()
			window := s.currentWindow("2025-02-22T12:04:05Z")
			Expect(window).To(BeNil())
		})
	})

	Context("window with single value", func() {
		It("returns empty window if current time not in grace period", func() {
			s.forEmptySlidingWindow()
			s.addValue(1234, "2025-02-22T12:04:04Z")
			window := s.currentWindow("2025-02-22T12:04:05Z")
			Expect(window).To(BeNil())
		})

		It("returns window if current time falls into grace period", func() {
			s.forEmptySlidingWindow()
			s.addValue(1234, "2025-02-22T12:03:05Z")
			window := s.currentWindow("2025-02-22T12:04:05Z")
			Expect(window).NotTo(BeNil())
		})
	})
})

type sutslidingwindow struct {
	slidingWindow *servicelevels.SlidingWindow
}

func (s *sutslidingwindow) forEmptySlidingWindow() {
	s.slidingWindow = &servicelevels.SlidingWindow{
		Size:        time.Minute,
		GracePeriod: 10 * time.Second,
	}
}

func (s *sutslidingwindow) currentWindow(nowString string) interface{} {
	now, parseErr := time.Parse(time.RFC3339, nowString)
	Expect(parseErr).NotTo(HaveOccurred())
	return s.slidingWindow.GetWindow(now)
}

func (s *sutslidingwindow) addValue(latency float64, nowString string) {
	now, parseErr := time.Parse(time.RFC3339, nowString)
	Expect(parseErr).NotTo(HaveOccurred())
	s.slidingWindow.AddValue(now, latency)
}
