package servicelevels

import "time"

type Window struct {
	StartTime   time.Time
	EndTime     time.Time
	Accumulator Accumulator
}

func (w *Window) IsActive(now time.Time) bool {
	return (now.After(w.StartTime) || now.Equal(w.StartTime)) &&
		(now.Before(w.EndTime) || now.Equal(w.EndTime))
}

func (w *Window) IsInFuture(now time.Time) bool {
	return now.Before(w.StartTime)
}

func (w *Window) IsActiveGracePeriod(now time.Time, gracePeriod time.Duration) bool {
	graceEnd := w.EndTime
	graceStart := w.EndTime.Add(-gracePeriod)

	return (now.After(graceStart) || now.Equal(graceStart)) &&
		(now.Before(graceEnd) || now.Equal(graceEnd))
}

type Accumulator interface {
	Add(value float64)
}

type SlidingWindow struct {
	Size        time.Duration
	GracePeriod time.Duration
	windows     map[time.Time]*Window
	createAcc   func() Accumulator
}

func NewSlidingWindow(createAcc func() Accumulator, size time.Duration, gracePeriod time.Duration) *SlidingWindow {
	return &SlidingWindow{
		Size:        size,
		GracePeriod: gracePeriod,
		createAcc:   createAcc,
		windows:     make(map[time.Time]*Window),
	}
}

func (w *SlidingWindow) GetActiveWindow(now time.Time) *Window {
	if len(w.windows) == 0 {
		return nil
	}
	w.pruneInactiveWindows(now)
	for _, window := range w.windows {
		if window.IsActiveGracePeriod(now, w.GracePeriod) {
			return window
		}
	}
	return nil
}

func (w *SlidingWindow) pruneInactiveWindows(now time.Time) {
	for key, window := range w.windows {
		if !(window.IsActive(now) || window.IsInFuture(now)) {
			delete(w.windows, key)
		}
	}
}

func (w *SlidingWindow) AddValue(now time.Time, value float64) {
	w.pruneInactiveWindows(now)

	for offset := time.Duration(0); offset <= w.Size; offset += w.GracePeriod {
		startTime := now.Truncate(w.GracePeriod).Add(offset)
		endTime := startTime.Add(w.Size)

		_, found := w.windows[startTime]
		if !found {
			w.windows[startTime] = &Window{
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
