package servicelevels

import "time"

type Window struct {
	StartTime   time.Time
	EndTime     time.Time
	Accumulator Accumulator
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

	window := w.windows[0]
	graceEnd := window.EndTime
	graceStart := window.EndTime.Add(-w.GracePeriod)

	if (now.After(graceStart) || now.Equal(graceStart)) &&
		(now.Before(graceEnd) || now.Equal(graceEnd)) {
		return &window
	}
	return nil
}

func (w *SlidingWindow) AddValue(now time.Time, value float64) {
	if w.windows == nil {

		startTime := now.Truncate(w.GracePeriod)
		endTime := startTime.Add(w.Size)
		
		w.windows = []Window{
			{
				StartTime:   startTime,
				EndTime:     endTime,
				Accumulator: w.createAcc(),
			},
		}
	}
	w.windows[0].Accumulator.Add(value)
}
