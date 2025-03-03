package servicelevels

import "errors"

var P50, _ = ParsePercentile(50)
var P70, _ = ParsePercentile(70)
var P90, _ = ParsePercentile(90)
var P99, _ = ParsePercentile(99)
var P999, _ = ParsePercentile(99.9)

type Percentile float64

func ParsePercentile(value float64) (Percentile, error) {
	if value > 0 && value <= 100.0 {
		return Percentile(value / 100.0), nil
	}
	return Percentile(0), errors.New("value out of range")
}

func (p *Percentile) Normalized() float64 {
	return float64(*p)
}

type LatencyMs int64

type Percent float64

func ParsePercent(value float64) (Percent, error) {
	if value > 0 && value <= 100.0 {
		return Percent(value), nil
	}
	return Percent(0), errors.New("value out of range")
}

func (p *Percent) Value() float64 {
	return float64(*p)
}
