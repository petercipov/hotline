package proxy_test

import (
	"sync"
	"time"

	"hotline/proxy"
)

// fakeClock runs with the real time, but its waits end only when the test
// lets time pass.
type fakeClock struct {
	mu     sync.Mutex
	offset time.Duration
	timers []*fakeTimer
}

type fakeTimer struct {
	clock *fakeClock
	at    time.Time
	f     func()
	done  bool
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return time.Now().Add(c.offset)
}

func (c *fakeClock) AfterFunc(d time.Duration, f func()) proxy.Timer {
	t := &fakeTimer{clock: c, at: c.Now().Add(d), f: f}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.timers = append(c.timers, t)
	return t
}

func (t *fakeTimer) Stop() bool {
	t.clock.mu.Lock()
	defer t.clock.mu.Unlock()
	stopped := !t.done
	t.done = true
	return stopped
}

// pass lets d go by, firing every wait that ends on the way. It says how
// many fired.
func (c *fakeClock) pass(d time.Duration) int {
	c.mu.Lock()
	c.offset += d
	now := time.Now().Add(c.offset)
	var due []func()
	for _, t := range c.timers {
		if !t.done && !t.at.After(now) {
			t.done = true
			due = append(due, t.f)
		}
	}
	c.mu.Unlock()
	for _, f := range due {
		f()
	}
	return len(due)
}
