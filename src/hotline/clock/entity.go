package clock

import "time"

type NowFunc func() time.Time

type ManagedTime interface {
	Now() time.Time
	Sleep(time.Duration)
	TickPeriodically(duration time.Duration, handler func(now time.Time)) func()

	AfterFunc(d time.Duration, f func(now time.Time))
}

func ParseTime(nowString string) time.Time {
	now, _ := time.Parse(time.RFC3339, nowString)
	return now
}
