package servicelevels

import "time"

type Window struct {
	StartTime time.Time
	EndTime   time.Time
}

type SlidingWindow struct {
	Size        time.Duration
	GracePeriod time.Duration
	windows     []Window
}

func (w *SlidingWindow) GetActiveWindow(now time.Time) *Window {
	if w.windows == nil {
		return nil
	}

	window := w.windows[0]
	graceEnd := window.EndTime
	graceStart := window.EndTime.Add(-w.GracePeriod)

	if (now.After(graceStart) || now.Equal(graceStart)) &&
		(now.Before(graceEnd) || now.Equal(graceEnd)) {
		return &window
	}
	return nil
}

func (w *SlidingWindow) AddValue(now time.Time, _ float64) {
	if w.windows == nil {
		w.windows = []Window{
			{
				StartTime: now,
				EndTime:   now.Add(w.Size),
			},
		}
	}
}
