package config

import (
	"encoding/json"
	"hotline/http"
	"hotline/servicelevels"
	"time"
)

type Duration time.Duration

func (d *Duration) toMs() int64 {
	return time.Duration(*d).Milliseconds()
}

func (d *Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(*d).String())
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

func ParseServiceLevelFromBytes(data []byte) (servicelevels.HttpApiSLODefinition, error) {
	var config ListDefinitions
	unmarshalErr := json.Unmarshal(data, &config)
	if unmarshalErr != nil {
		return servicelevels.HttpApiSLODefinition{}, unmarshalErr
	}
	return ParseServiceLevel(config)
}

func ParseServiceLevel(config ListDefinitions) (servicelevels.HttpApiSLODefinition, error) {
	result := servicelevels.HttpApiSLODefinition{
		RouteSLOs: make([]servicelevels.HttpRouteSLODefinition, len(config.Routes)),
	}

	for i, route := range config.Routes {
		percentile := route.Definition.Status.BreachThreshold.Cast()
		breachThreshold, breachErr := servicelevels.ParsePercent(percentile.Normalized() * 100)
		if breachErr != nil {
			return servicelevels.HttpApiSLODefinition{}, breachErr
		}

		defs, defsErr := parsePercentileDefinitions(route.Definition.Latency.Percentiles)
		if defsErr != nil {
			return servicelevels.HttpApiSLODefinition{}, defsErr
		}

		result.RouteSLOs[i] = servicelevels.HttpRouteSLODefinition{
			Route: http.Route{
				Method:      string(route.Route.Method),
				PathPattern: route.Route.Path,
				Host:        route.Route.Host,
				Port:        route.Route.Port,
			},
			Latency: servicelevels.HttpLatencySLODefinition{
				Percentiles:    defs,
				WindowDuration: time.Duration(route.Definition.Latency.WindowDuration),
			},
			Status: servicelevels.HttpStatusSLODefinition{
				Expected:        route.Definition.Status.Expected,
				BreachThreshold: breachThreshold,
				WindowDuration:  time.Duration(route.Definition.Status.WindowDuration),
			},
		}
	}

	return result, nil
}

func parsePercentileDefinitions(percentiles []PercentileThreshold) ([]servicelevels.PercentileDefinition, error) {
	result := make([]servicelevels.PercentileDefinition, len(percentiles))

	for i, percentile := range percentiles {
		percentileValue := percentile.Percentile.Cast()
		result[i] = servicelevels.PercentileDefinition{
			Percentile: *percentileValue,
			Threshold:  servicelevels.LatencyMs(percentile.BreachThreshold.toMs()),
			Name:       percentileValue.Name(),
		}
	}
	return result, nil
}
