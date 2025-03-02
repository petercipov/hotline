package servicelevels

import "time"

type SLOCheck struct {
	Metric    Metric
	Breakdown []Metric
	Breach    *SLOBreach
}

type Metric struct {
	Name  string
	Value float64
}

type Operation string

var OperationGTE Operation = ">="
var OperationLE Operation = "<"

type SLOBreach struct {
	Threshold      float64
	Operation      Operation
	WindowDuration time.Duration
}
