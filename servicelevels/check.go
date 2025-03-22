package servicelevels

import "time"

type SLOCheck struct {
	Metric    Metric
	Tags      map[string]string
	Breakdown []Metric
	Breach    *SLOBreach
}

type Metric struct {
	Name  string
	Value float64
	Unit  string
}

type Operation string

var OperationGE Operation = ">="
var OperationL Operation = "<"

type SLOBreach struct {
	ThresholdValue float64
	ThresholdUnit  string
	Operation      Operation
	WindowDuration time.Duration
}
