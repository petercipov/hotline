package servicelevels

import "time"

type Window struct {
	StartTime   time.Time
	EndTime     time.Time
	Accumulator Accumulator
}

func (w *Window) IsActive(now time.Time) bool {
	nowSec := now.Unix()
	startSec := w.StartTime.Unix()
	endSec := w.EndTime.Unix()

	return nowSec >= startSec && nowSec < endSec
}

func (w *Window) IsInFuture(now time.Time) bool {
	nowSec := now.Unix()
	startSec := w.StartTime.Unix()
	return nowSec < startSec
}

func (w *Window) IsActiveGracePeriod(now time.Time, gracePeriod time.Duration) bool {
	graceEnd := w.EndTime
	graceStart := w.EndTime.Add(-gracePeriod)

	nowSec := now.Unix()
	graceStartSec := graceStart.Unix()
	graceEndSec := graceEnd.Unix()

	return nowSec >= graceStartSec && nowSec < graceEndSec
}

func (w *Window) IsObsolete(now time.Time) bool {
	return !(w.IsActive(now) || w.IsInFuture(now))
}

type Accumulator interface {
	Add(value float64)
}

type SlidingWindow struct {
	Size        time.Duration
	GracePeriod time.Duration
	windows     map[int64]*Window
	createAcc   func() Accumulator
}

func NewSlidingWindow(createAcc func() Accumulator, size time.Duration, gracePeriod time.Duration) *SlidingWindow {
	return &SlidingWindow{
		Size:        size,
		GracePeriod: gracePeriod,
		createAcc:   createAcc,
		windows:     make(map[int64]*Window),
	}
}

func (w *SlidingWindow) GetActiveWindow(now time.Time) *Window {
	if len(w.windows) == 0 {
		return nil
	}
	w.pruneObsoleteWindows(now)
	for _, window := range w.windows {
		if window.IsActiveGracePeriod(now, w.GracePeriod) {
			return window
		}
	}
	return nil
}

func (w *SlidingWindow) pruneObsoleteWindows(now time.Time) {
	for key, window := range w.windows {
		if window.IsObsolete(now) {
			delete(w.windows, key)
		}
	}
}

func (w *SlidingWindow) AddValue(now time.Time, value float64) {
	w.pruneObsoleteWindows(now)

	for offset := time.Duration(0); offset <= w.Size; offset += w.GracePeriod {
		startTime := now.Truncate(w.GracePeriod).Add(offset)
		endTime := startTime.Add(w.Size)

		key := startTime.Unix()
		_, found := w.windows[key]
		if !found {
			w.windows[key] = &Window{
				StartTime:   startTime,
				EndTime:     endTime,
				Accumulator: w.createAcc(),
			}
		}
	}

	for key := range w.windows {
		w.windows[key].Accumulator.Add(value)
	}
}
