package servicelevels

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

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
	return Percentile(0), errors.New("value out of range >0 and <=100")
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
