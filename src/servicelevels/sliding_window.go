package servicelevels

import "time"

type Window[T any] struct {
	StartTime   time.Time
	EndTime     time.Time
	Accumulator Accumulator[T]
}

func (w *Window[T]) IsActive(now time.Time) bool {
	nowSec := now.Unix()
	startSec := w.StartTime.Unix()
	endSec := w.EndTime.Unix()

	return nowSec >= startSec && nowSec < endSec
}

func (w *Window[T]) IsActiveGracePeriod(now time.Time, gracePeriod time.Duration) bool {
	graceEnd := w.EndTime
	graceStart := w.EndTime.Add(-gracePeriod)

	nowSec := now.Unix()
	graceStartSec := graceStart.Unix()
	graceEndSec := graceEnd.Unix()

	return nowSec >= graceStartSec && nowSec < graceEndSec
}

type Accumulator[T any] interface {
	Add(value T)
}

type SlidingWindow[T any] struct {
	Size        time.Duration
	GracePeriod time.Duration
	windows     map[int64]*Window[T]
	createAcc   func() Accumulator[T]
}

func NewSlidingWindow[T any](createAcc func() Accumulator[T], size time.Duration, gracePeriod time.Duration) *SlidingWindow[T] {
	return &SlidingWindow[T]{
		Size:        size,
		GracePeriod: gracePeriod,
		createAcc:   createAcc,
		windows:     make(map[int64]*Window[T]),
	}
}

func (w *SlidingWindow[T]) GetActiveWindow(now time.Time) *Window[T] {
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

func (w *SlidingWindow[T]) pruneObsoleteWindows(now time.Time) {
	for key, window := range w.windows {
		if !window.IsActive(now) {
			delete(w.windows, key)
		}
	}
}

func (w *SlidingWindow[T]) AddValue(now time.Time, value T) {
	w.pruneObsoleteWindows(now)

	windowStart := now.Truncate(w.GracePeriod)
	for offset := time.Duration(0); offset <= w.Size; offset += w.GracePeriod {
		startTime := windowStart.Add(-offset)
		endTime := startTime.Add(w.Size)

		key := startTime.Unix()
		_, found := w.windows[key]
		if !found {
			w.windows[key] = &Window[T]{
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
