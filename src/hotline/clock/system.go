package clock

import "time"

type SystemClock struct {
}

func NewSystemClock() *SystemClock {
	return &SystemClock{}
}

func (m *SystemClock) Now() time.Time {
	return time.Now()
}

func (m *SystemClock) Sleep(d time.Duration) {
	time.Sleep(d)
}

func (m *SystemClock) TickPeriodically(duration time.Duration, handler func(t time.Time)) func() {
	ticker := time.NewTicker(duration)
	go func() {
		for t := range ticker.C {
			handler(t)
		}
	}()
	return ticker.Stop
}

func (m *SystemClock) AfterFunc(d time.Duration, f func(now time.Time)) {
	time.AfterFunc(d, func() {
		f(time.Now())
	})
}
