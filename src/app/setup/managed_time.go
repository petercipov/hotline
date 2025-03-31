package setup

import (
	"sync"
	"time"
)

type ManagedTime interface {
	Now() time.Time
	Sleep(time.Duration)
	TickPeriodically(duration time.Duration, handler func(t time.Time)) func()
}

type SystemTime struct {
}

func NewSystemTime() *SystemTime {
	return &SystemTime{}
}

func (m *SystemTime) Now() time.Time {
	return time.Now()
}

func (m *SystemTime) Sleep(d time.Duration) {
	time.Sleep(d)
}

func (m *SystemTime) TickPeriodically(duration time.Duration, handler func(t time.Time)) func() {
	ticker := time.NewTicker(duration)
	go func() {
		for t := range ticker.C {
			handler(t)
		}
	}()
	return ticker.Stop
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

type ManualTime struct {
	now     time.Time
	tickers map[*timeKey]*manualTicker
	s       *sync.Mutex
}

func NewManualTime(now time.Time) *ManualTime {
	return &ManualTime{
		now:     now,
		tickers: make(map[*timeKey]*manualTicker),
		s:       &sync.Mutex{},
	}
}

func (t *ManualTime) Now() time.Time {
	t.s.Lock()
	currentTime := t.now
	t.s.Unlock()

	return currentTime
}

func (t *ManualTime) Sleep(_ time.Duration) {
	// nothing
}

func (t *ManualTime) Advance(duration time.Duration) {
	t.s.Lock()
	t.now = t.now.Add(duration)
	currentTime := t.now
	currentTickers := t.tickers
	t.s.Unlock()

	for _, ticker := range currentTickers {
		ticker.tick(currentTime)
	}
}

func (t *ManualTime) TickPeriodically(duration time.Duration, handler func(t time.Time)) func() {
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
