package servicelevels

var P50, _ = ParsePercentile(50)
var P70, _ = ParsePercentile(70)
var P90, _ = ParsePercentile(90)
var P99, _ = ParsePercentile(99)
var P999, _ = ParsePercentile(99.9)

type Percentile float64

func ParsePercentile(value float64) (Percentile, bool) {
	if value > 0 && value <= 100.0 {
		return Percentile(value / 100.0), false
	}
	return Percentile(0), true
}

func (p *Percentile) Normalized() float64 {
	return float64(*p)
}

type LatencyMs int64

type Percent float64

func ParsePercent(value float64) (Percent, bool) {
	if value > 0 && value <= 100.0 {
		return Percent(value), false
	}
	return Percent(0), true
}
