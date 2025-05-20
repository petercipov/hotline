package clock_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"hotline/clock"
	"sync"
	"time"
)

var _ = Describe("Manual Clock", func() {
	sut := manualClockSUT{}

	It("should execute after function after given duration", func() {
		sut.ForSystemClock()
		now := sut.Now()
		afterTime := sut.After()
		Expect(afterTime.After(now)).To(BeTrue())
	})

	It("should sleep for given time", func() {
		sut.ForSystemClock()
		now := sut.Now()
		afterSleep := sut.Sleep()
		Expect(afterSleep.After(now)).To(BeTrue())
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

type manualClockSUT struct {
	clock *clock.ManualClock
}

func (s *manualClockSUT) ForSystemClock() {
	s.clock = clock.NewManualClock(time.Time{})
}

func (s *manualClockSUT) Sleep() time.Time {
	duration := 1 * time.Millisecond
	go func() {
		s.Advance(duration)
	}()
	s.clock.Sleep(duration)
	return s.clock.Now()
}

func (s *manualClockSUT) Now() time.Time {
	return s.clock.Now()
}

func (s *manualClockSUT) Advance(d time.Duration) time.Time {
	s.clock.Advance(d)
	return s.clock.Now()
}

func (s *manualClockSUT) After() time.Time {
	var afterFunc time.Time
	var w sync.WaitGroup
	w.Add(1)
	s.clock.AfterFunc(1*time.Millisecond, func(after time.Time) {
		afterFunc = after
		w.Done()
	})
	go func() {
		s.clock.Advance(1 * time.Millisecond)
	}()
	w.Wait()
	return afterFunc
}

func (s *manualClockSUT) TickPeriodically(n int) []time.Time {
	var ticks []time.Time

	var w sync.WaitGroup
	w.Add(n)

	go func() {
		s.clock.Advance(time.Duration(n) * time.Millisecond)
	}()

	cancel := s.clock.TickPeriodically(1*time.Millisecond, func(t time.Time) {
		ticks = append(ticks, t)
		w.Done()
	})
	w.Wait()
	cancel()
	return ticks
}
