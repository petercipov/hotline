package clock

import (
	"sync"
	"time"
)

type ManualClock struct {
	now     time.Time
	tickers map[*timeKey]*manualTicker
	s       *sync.Mutex
}

func NewManualClock(now time.Time) *ManualClock {
	return &ManualClock{
		now:     now,
		tickers: make(map[*timeKey]*manualTicker),
		s:       &sync.Mutex{},
	}
}

type timeKey struct{}
type manualTicker struct {
	handler       func(now time.Time)
	monotonicTime time.Time
	duration      time.Duration
}

func (m *manualTicker) tick(now time.Time) {
	tick := m.monotonicTime.Add(m.duration)
	for tick.Before(now) || tick.Equal(now) {
		m.monotonicTime = tick
		m.handler(m.monotonicTime)
		tick = m.monotonicTime.Add(m.duration)
	}
}

func (t *ManualClock) Now() time.Time {
	t.s.Lock()
	currentTime := t.now
	t.s.Unlock()

	return currentTime
}

func (t *ManualClock) Sleep(d time.Duration) {
	var w sync.WaitGroup
	w.Add(1)
	t.AfterFunc(d, func(_ time.Time) {
		w.Done()
	})
	w.Wait()
}

func (t *ManualClock) Advance(duration time.Duration) {
	t.s.Lock()
	t.now = t.now.Add(duration)
	currentTime := t.now
	currentTickers := t.tickers
	t.s.Unlock()

	for _, ticker := range currentTickers {
		ticker.tick(currentTime)
	}
}

func (t *ManualClock) TickPeriodically(duration time.Duration, handler func(t time.Time)) func() {
	key := &timeKey{}
	now := t.Now()
	t.s.Lock()
	t.tickers[key] = &manualTicker{
		handler:       handler,
		monotonicTime: now,
		duration:      duration,
	}
	t.s.Unlock()

	return func() {
		t.s.Lock()
		delete(t.tickers, key)
		t.s.Unlock()
	}
}

func (t *ManualClock) AfterFunc(duration time.Duration, f func(now time.Time)) {
	key := &timeKey{}
	now := t.Now()
	t.s.Lock()
	t.tickers[key] = &manualTicker{
		handler: func(now time.Time) {
			t.s.Lock()
			delete(t.tickers, key)
			t.s.Unlock()
			f(now)
		},
		monotonicTime: now,
		duration:      duration,
	}
	t.s.Unlock()
}
