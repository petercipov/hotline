package config

import (
	"encoding/json"
	"hotline/http"
	"hotline/servicelevels"
	"time"
)

type Duration time.Duration

func (d Duration) toMs() int64 {
	return time.Duration(d).Milliseconds()
}

func (d Duration) MarshalJSON() ([]byte, error) {
	tVal := time.Duration(d).String()
	return json.Marshal(tVal)
}

func (d *Duration) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	val, err := time.ParseDuration(str)
	*d = Duration(val)
	return err
}

type Percentile servicelevels.Percentile

func (p Percentile) MarshalJSON() ([]byte, error) {
	str := p.Cast().AsValue()
	return json.Marshal(str)
}

func (p *Percentile) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}

	val, err := servicelevels.ParsePercentileFromValue(str)
	if err != nil {
		return err
	}
	*p = Percentile(val)
	return nil
}

func (p *Percentile) Cast() *servicelevels.Percentile {
	return (*servicelevels.Percentile)(p)
}

func ParseRoute(definition SLODefinition, route Route) (servicelevels.HttpRouteSLODefinition, error) {
	percentile := definition.Status.BreachThreshold.Cast()

	defs, defsErr := parsePercentileDefinitions(definition.Latency.Percentiles)
	if defsErr != nil {
		return servicelevels.HttpRouteSLODefinition{}, defsErr
	}

	return servicelevels.HttpRouteSLODefinition{
		Route: http.Route{
			Method:      string(route.Method),
			PathPattern: route.Path,
			Host:        route.Host,
			Port:        route.Port,
		},
		Latency: servicelevels.HttpLatencySLODefinition{
			Percentiles:    defs,
			WindowDuration: time.Duration(definition.Latency.WindowDuration),
		},
		Status: servicelevels.HttpStatusSLODefinition{
			Expected:        definition.Status.Expected,
			BreachThreshold: *percentile,
			WindowDuration:  time.Duration(definition.Status.WindowDuration),
		},
	}, nil
}

func parsePercentileDefinitions(percentiles []PercentileThreshold) ([]servicelevels.PercentileDefinition, error) {
	result := make([]servicelevels.PercentileDefinition, len(percentiles))

	for i, percentile := range percentiles {
		percentileValue := percentile.Percentile.Cast()
		result[i] = servicelevels.PercentileDefinition{
			Percentile: *percentileValue,
			Threshold:  servicelevels.LatencyMs(percentile.BreachLatency.toMs()),
			Name:       percentileValue.Name(),
		}
	}
	return result, nil
}
