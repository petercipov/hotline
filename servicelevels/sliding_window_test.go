package servicelevels_test

import (
	"fmt"
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
			s.addValue(1234, "2025-02-22T12:03:05Z")
			window := s.getActiveWindow("2025-02-22T12:04:00Z")
			Expect(window).To(BeNil())
		})

		It("returns active window if current time falls into grace period", func() {
			s.forEmptySlidingWindow()
			s.addValue(1234, "2025-02-22T12:03:05Z")
			window := s.getActiveWindow("2025-02-22T12:03:59Z")
			Expect(window).NotTo(BeNil())
		})

		It("returns active window containing inserted value", func() {
			s.forEmptySlidingWindow()
			s.addValue(1234, "2025-02-22T12:03:05Z")
			window := s.getActiveWindow("2025-02-22T12:03:59Z")
			Expect(window).NotTo(BeNil())
			Expect(s.windowContains(window, 1234)).To(BeTrue())
		})

		It("should generate multiple windows to past from single data, scrolled by grace period", func() {
			s.forEmptySlidingWindow()
			s.addValue(1234, "2025-02-22T12:03:05Z")

			scrolledWindows := s.scrollByGracePeriod("2025-02-22T12:03:05Z", 7)
			startTimes := scrolledWindows.StartTimes()
			Expect(startTimes).To(Equal([]*time.Time{
				parseTimePtr("2025-02-22T12:02:10Z"),
				parseTimePtr("2025-02-22T12:02:20Z"),
				parseTimePtr("2025-02-22T12:02:30Z"),
				parseTimePtr("2025-02-22T12:02:40Z"),
				parseTimePtr("2025-02-22T12:02:50Z"),
				parseTimePtr("2025-02-22T12:03:00Z"),
				nil,
			}))
		})

		It("should generate multiple windows in past and future for 2 data points, window size away, from each other", func() {
			s.forEmptySlidingWindow()
			s.addValue(1234, "2025-02-22T12:03:05Z")
			s.addValue(1234, "2025-02-22T12:04:05Z")
			scrolledWindows := s.scrollByGracePeriod("2025-02-22T12:04:05Z", 7)
			startTimes := scrolledWindows.StartTimes()

			Expect(startTimes).To(Equal([]*time.Time{
				parseTimePtr("2025-02-22T12:03:10Z"),
				parseTimePtr("2025-02-22T12:03:20Z"),
				parseTimePtr("2025-02-22T12:03:30Z"),
				parseTimePtr("2025-02-22T12:03:40Z"),
				parseTimePtr("2025-02-22T12:03:50Z"),
				parseTimePtr("2025-02-22T12:04:00Z"),
				nil,
			}))
		})
	})

	Context("window with multiple values", func() {
		It("values are shared if windows overlap", func() {
			s.forEmptySlidingWindow()
			s.addValue(1234, "2025-02-22T12:04:05Z")
			s.addValue(2345, "2025-02-22T12:04:15Z")

			window := s.getActiveWindow("2025-02-22T12:04:50Z")
			Expect(window).NotTo(BeNil())
			Expect(s.windowContains(window, 1234)).To(BeTrue())
			Expect(s.windowContains(window, 2345)).To(BeTrue())
		})

		It("hops to next window if first value if out of  window boundaries", func() {
			s.forEmptySlidingWindow()
			s.addValue(1234, "2025-02-22T12:04:04Z")
			s.addValue(2345, "2025-02-22T12:05:10Z")

			window := s.getActiveWindow("2025-02-22T12:06:05Z")
			Expect(window).NotTo(BeNil())
			Expect(s.windowContains(window, 1234)).NotTo(BeTrue())
			Expect(s.windowContains(window, 2345)).To(BeTrue())
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

func parseTimePtr(nowString string) *time.Time {
	now := parseTime(nowString)
	return &now
}

func parseTime(nowString string) time.Time {
	now, parseErr := time.Parse(time.RFC3339, nowString)
	Expect(parseErr).NotTo(HaveOccurred())
	return now
}

func (s *sutslidingwindow) getActiveWindow(nowString string) *servicelevels.Window {
	now := parseTime(nowString)
	return s.slidingWindow.GetActiveWindow(now)
}

func (s *sutslidingwindow) addValue(latency float64, nowString string) {
	now := parseTime(nowString)
	s.slidingWindow.AddValue(now, latency)
}

func (s *sutslidingwindow) windowContains(window *servicelevels.Window, value float64) bool {
	acc := window.Accumulator.(*arrAccumulator)
	fmt.Printf("%+v", acc.values)
	return slices.Contains(acc.values, value)
}

func (s *sutslidingwindow) scrollByGracePeriod(nowStr string, count int) scrolledWindows {
	now := parseTime(nowStr)
	var windows []*servicelevels.Window
	for i := 0; i < count; i++ {
		tNow := now.Add(s.slidingWindow.GracePeriod * time.Duration(i))
		window := s.slidingWindow.GetActiveWindow(tNow)
		windows = append(windows, window)
	}
	return windows
}

type scrolledWindows []*servicelevels.Window

func (s *scrolledWindows) StartTimes() []*time.Time {
	if s == nil {
		return nil
	}

	var startTimes []*time.Time
	for _, window := range *s {
		if window == nil {
			startTimes = append(startTimes, nil)
		} else {
			startTimes = append(startTimes, &window.StartTime)
		}
	}

	return startTimes
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
