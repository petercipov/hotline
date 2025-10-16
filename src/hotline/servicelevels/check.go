package servicelevels

import "time"

type LevelsCheck struct {
	Namespace string
	Metric    Metric
	Tags      map[string]string
	Breakdown []Metric
	Timestamp time.Time
	Uptime    time.Duration
}

type Metric struct {
	Name        string
	Value       float64
	Unit        string
	EventsCount int64
}
