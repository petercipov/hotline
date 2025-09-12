package servicelevels

import "time"

type SLOCheck struct {
	Namespace string
	Metric    Metric
	Tags      map[string]string
	Breakdown []Metric
	Breach    *SLOBreach
}

type Metric struct {
	Name        string
	Value       float64
	Unit        string
	EventsCount int64
}

type Operation string

const OperationGE = Operation(">=")
const OperationL = Operation("<")

type SLOBreach struct {
	ThresholdValue float64
	ThresholdUnit  string
	Operation      Operation
	WindowDuration time.Duration
}
