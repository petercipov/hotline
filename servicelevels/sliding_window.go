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

func (w *Window) IsActiveInGracePeriod(now time.Time, gracePeriod time.Duration) bool {
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
	windows     []Window
	createAcc   func() Accumulator
}

func NewSlidingWindow(createAcc func() Accumulator, size time.Duration, gracePeriod time.Duration) *SlidingWindow {
	return &SlidingWindow{
		Size:        size,
		GracePeriod: gracePeriod,
		createAcc:   createAcc,
	}
}

func (w *SlidingWindow) GetActiveWindow(now time.Time) *Window {
	if w.windows == nil {
		return nil
	}

	w.pruneInactiveWindows(now)
	for _, window := range w.windows {
		if window.IsActiveInGracePeriod(now, w.GracePeriod) {
			return &window
		}
	}
	return nil
}

func (w *SlidingWindow) pruneInactiveWindows(now time.Time) {
	var activeWindows []Window
	for _, window := range w.windows {
		if window.IsActive(now) {
			activeWindows = append(activeWindows, window)
		}
	}
	w.windows = activeWindows
}

func (w *SlidingWindow) AddValue(now time.Time, value float64) {
	w.pruneInactiveWindows(now)

	for offset := time.Duration(0); offset <= w.Size; offset += w.GracePeriod {
		startTime := now.Truncate(w.GracePeriod).Add(-offset)
		endTime := startTime.Add(w.Size)

		found := false
		for _, window := range w.windows {
			if window.StartTime.Equal(startTime) {
				found = true
			}
		}

		if !found {
			w.windows = append(w.windows, Window{
				StartTime:   startTime,
				EndTime:     endTime,
				Accumulator: w.createAcc(),
			})
		}
	}

	for i := range w.windows {
		w.windows[i].Accumulator.Add(value)
	}
}
