package clock_test

import (
	"hotline/clock"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Manual Clock", Ordered, func() {
	sut := manualClockSUT{}

	AfterEach(func() {
		sut.clock = nil
	})

	It("should execute after function after given duration", func() {
		sut.ForManualClock()
		now := sut.Now()
		afterTime := sut.After()
		Expect(afterTime.After(now)).To(BeTrue())
	})

	It("should sleep for given time", func() {
		sut.ForManualClock()
		now := sut.Now()
		afterSleep := sut.Sleep()
		Expect(afterSleep.After(now)).To(BeTrue())
	})

	It("should tick periodically", func() {
		sut.ForManualClock()
		now := sut.Now()
		ticks := sut.TickPeriodicallyAndCancel(10)
		Expect(len(ticks)).To(Equal(10))

		for _, tick := range ticks {
			Expect(tick.After(now)).To(BeTrue())
			now = tick
		}
	})

	It("should reset periodical tickers", func() {
		sut.ForManualClock()
		starTime := sut.Now()

		ticks := sut.TickPeriodicallyAndResetOnce(10, starTime)
		Expect(len(ticks)).To(Equal(20))

		for i := 0; i < 10; i++ {
			Expect(ticks[i]).To(Equal(ticks[i+10]))
		}
	})

	It("should not advance automatically if not set", func() {
		sut.ForManualClock()

		now := sut.Now()
		next := sut.Now()

		Expect(now).To(Equal(next))
	})

	It("should advance automatically if set", func() {
		sut.WithAutoAdvance()

		now := sut.Now()
		next := sut.Now()

		Expect(next.After(now)).To(BeTrue())
	})
})

type manualClockSUT struct {
	clock *clock.ManualClock
}

func (s *manualClockSUT) ForManualClock() {
	s.clock = clock.NewManualClock(clock.ParseTime("2025-05-18T12:02:10Z"), 0)
}

func (s *manualClockSUT) WithAutoAdvance() {
	s.clock = clock.NewManualClock(clock.ParseTime("2025-05-18T12:02:10Z"), 1)
}

func (s *manualClockSUT) Sleep() time.Time {
	s.clock.Sleep(1 * time.Millisecond)
	return s.clock.Now()
}

func (s *manualClockSUT) Now() time.Time {
	return s.clock.Now()
}

func (s *manualClockSUT) Advance(d time.Duration) time.Time {
	if s.clock == nil {
		return time.Time{}
	}
	s.clock.Advance(d)
	if s.clock == nil {
		return time.Time{}
	}
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

func (s *manualClockSUT) TickPeriodicallyAndCancel(n int) []time.Time {
	var ticks []time.Time

	var w sync.WaitGroup
	w.Add(n)

	cancel := s.clock.TickPeriodically(1*time.Millisecond, func(t time.Time) {
		ticks = append(ticks, t)
		w.Done()
	})

	go func() {
		s.clock.Advance(time.Duration(n) * time.Millisecond)
	}()

	w.Wait()
	cancel()
	return ticks
}

func (s *manualClockSUT) TickPeriodicallyAndResetOnce(n int, resetTime time.Time) []time.Time {
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
	w.Add(n)
	s.clock.Reset(resetTime)

	go func() {
		s.clock.Advance(time.Duration(n) * time.Millisecond)
	}()

	w.Wait()
	cancel()
	return ticks
}
