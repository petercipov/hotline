package clock_test

import (
	"hotline/clock"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("System Clock", func() {
	sut := systemClockSUT{}

	It("should sleep for given time", func() {
		sut.ForSystemClock()
		now := sut.Now()
		afterSleep := sut.Sleep()
		Expect(afterSleep.After(now)).To(BeTrue())
	})

	It("should execute after function after given duration", func() {
		sut.ForSystemClock()
		now := sut.Now()
		afterTime := sut.After()
		Expect(afterTime.After(now)).To(BeTrue())
	})

	It("should tick periodically", func() {
		sut.ForSystemClock()
		now := sut.Now()
		ticks := sut.TickPeriodically(10)
		Expect(len(ticks)).To(Equal(10))

		for _, tick := range ticks {
			Expect(tick.After(now)).To(BeTrue())
			now = tick
		}
	})
})

type systemClockSUT struct {
	clock *clock.SystemClock
}

func (s *systemClockSUT) ForSystemClock() {
	s.clock = clock.NewSystemClock()
}

func (s *systemClockSUT) Sleep() time.Time {
	s.clock.Sleep(1 * time.Millisecond)
	return s.clock.Now()
}

func (s *systemClockSUT) Now() time.Time {
	return s.clock.Now()
}

func (s *systemClockSUT) After() time.Time {
	var afterFunc time.Time
	var w sync.WaitGroup
	w.Add(1)
	s.clock.AfterFunc(1*time.Millisecond, func(after time.Time) {
		afterFunc = after
		w.Done()
	})
	w.Wait()
	return afterFunc
}

func (s *systemClockSUT) TickPeriodically(n int) []time.Time {
	var ticks []time.Time

	var w sync.WaitGroup
	w.Add(n)
	cancel := s.clock.TickPeriodically(1*time.Millisecond, func(t time.Time) {
		ticks = append(ticks, t)
		w.Done()
	})
	w.Wait()
	cancel()
	return ticks
}
