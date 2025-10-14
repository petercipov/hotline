package clock

import (
	"sync"
	"time"
)

type ManualClock struct {
	now          time.Time
	tickers      map[int64]*manualTicker
	tickerIDs    int64
	s            *sync.Mutex
	advanceOnNow time.Duration
}

func NewDefaultManualClock() *ManualClock {
	return NewManualClock(
		ParseTime("2025-02-22T12:02:10Z"),
		500*time.Microsecond,
	)
}

func NewManualClock(now time.Time, advanceOnNow time.Duration) *ManualClock {
	return &ManualClock{
		now:          now,
		tickers:      make(map[int64]*manualTicker),
		s:            &sync.Mutex{},
		advanceOnNow: advanceOnNow,
	}
}

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

func (m *manualTicker) reset(now time.Time) {
	m.monotonicTime = now
}

func (t *ManualClock) Now() time.Time {
	t.s.Lock()
	currentTime := t.now
	t.s.Unlock()

	if t.advanceOnNow > 0 {
		t.Advance(t.advanceOnNow)
	}

	return currentTime
}

func (t *ManualClock) Sleep(d time.Duration) {
	t.Advance(d)
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
	now := t.Now()
	t.s.Lock()
	t.tickerIDs++
	key := t.tickerIDs
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
	now := t.Now()
	t.s.Lock()
	t.tickerIDs++
	key := t.tickerIDs
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

func (t *ManualClock) Reset(now time.Time) {
	t.s.Lock()
	defer t.s.Unlock()

	t.now = now
	for _, ticker := range t.tickers {
		ticker.reset(t.now)
	}
}
