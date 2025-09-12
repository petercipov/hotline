package clock

import (
	"math"
	"time"
)

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

func TimeFromUint64OrZero(value uint64) time.Time {
	if value <= math.MaxInt64 {
		return time.Unix(0, int64(value)).UTC()
	}
	return time.Time{}
}

func TimeToUint64NanoOrZero(now time.Time) uint64 {
	value := now.UnixNano()
	valueToReturn := uint64(0)
	if value >= 0 {
		valueToReturn = uint64(value)
	}
	return valueToReturn
}
