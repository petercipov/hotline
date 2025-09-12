package servicelevels

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const P50 = Percentile(0.5)
const P70 = Percentile(0.7)
const P90 = Percentile(0.9)
const P99 = Percentile(0.99)
const P999 = Percentile(0.999)

type Percentile float64

var ErrPercentileOutOfRange = errors.New("value out of range >0 and <=100")

func ParsePercentile(value float64) (Percentile, error) {
	if value > 0 && value <= 100.0 {
		return Percentile(value / 100.0), nil
	}
	return Percentile(0), ErrPercentileOutOfRange
}

func ParsePercentileFromValue(value string) (Percentile, error) {
	strVal := strings.TrimSpace(strings.ReplaceAll(value, "%", ""))

	floatVal, parseErr := strconv.ParseFloat(strVal, 64)
	if parseErr != nil {
		return Percentile(0), parseErr
	}
	return ParsePercentile(floatVal)
}

func (p *Percentile) Normalized() float64 {
	return float64(*p)
}

func (p *Percentile) AsPercent() float64 {
	return p.Normalized() * 100.0
}

func (p *Percentile) Name() string {
	percentile := p.AsPercent()

	formattedStr := fmt.Sprintf("p%.5f", percentile)
	return strings.TrimRight(strings.TrimRight(formattedStr, "0"), ".")
}

func (p *Percentile) AsValue() string {
	percentile := p.AsPercent()

	formattedStr := fmt.Sprintf("%.5f", percentile)
	return strings.TrimRight(strings.TrimRight(formattedStr, "0"), ".") + "%"
}

type LatencyMs int64

func (l *LatencyMs) AsDuration() time.Duration {
	ms := int64(*l)
	return time.Duration(ms) * time.Millisecond
}
