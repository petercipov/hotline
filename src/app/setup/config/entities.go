package config

import (
	"encoding/json"
	"hotline/http"
	"hotline/servicelevels"
	"time"
)

type Duration time.Duration

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

type HttpApiSLOConfig struct {
	RouteSLOs []HttpRouteSLOConfig `json:"routeSLOs"`
}

type HttpRouteSLOConfig struct {
	Method  string               `json:"method,omitempty"`
	Path    string               `json:"path,omitempty"`
	Host    string               `json:"host,omitempty"`
	Port    int                  `json:"port,omitempty"`
	Latency HttpLatencySLOConfig `json:"latency"`
	Status  HttpStatusSLOConfig  `json:"status"`
}

type HttpLatencySLOConfig struct {
	Percentiles    []PercentileDefinition `json:"percentiles"`
	WindowDuration Duration               `json:"windowDuration"`
}

type HttpStatusSLOConfig struct {
	Expected        []string `json:"expected"`
	BreachThreshold float64  `json:"breachThreshold"`
	WindowDuration  Duration `json:"windowDuration"`
}

type PercentileDefinition struct {
	Percentile  float64 `json:"percentile"`
	ThresholdMs int64   `json:"thresholdMs"`
	Name        string  `json:"name"`
}

func ParseServiceLevelFromBytes(data []byte) (servicelevels.HttpApiSLODefinition, error) {
	var config HttpApiSLOConfig
	unmarshalErr := json.Unmarshal(data, &config)
	if unmarshalErr != nil {
		return servicelevels.HttpApiSLODefinition{}, unmarshalErr
	}
	return ParseServiceLevel(config)
}

func ParseServiceLevel(config HttpApiSLOConfig) (servicelevels.HttpApiSLODefinition, error) {
	result := servicelevels.HttpApiSLODefinition{
		RouteSLOs: make([]servicelevels.HttpRouteSLODefinition, len(config.RouteSLOs)),
	}

	for i, routeSLO := range config.RouteSLOs {

		breachThreshold, breachErr := servicelevels.ParsePercent(routeSLO.Status.BreachThreshold)
		if breachErr != nil {
			return servicelevels.HttpApiSLODefinition{}, breachErr
		}

		defs, defsErr := parsePercentileDefinitions(routeSLO.Latency.Percentiles)
		if defsErr != nil {
			return servicelevels.HttpApiSLODefinition{}, defsErr
		}

		result.RouteSLOs[i] = servicelevels.HttpRouteSLODefinition{
			Route: http.Route{
				Method:      routeSLO.Method,
				PathPattern: routeSLO.Path,
				Host:        routeSLO.Host,
				Port:        routeSLO.Port,
			},
			Latency: servicelevels.HttpLatencySLODefinition{
				Percentiles:    defs,
				WindowDuration: time.Duration(routeSLO.Latency.WindowDuration),
			},
			Status: servicelevels.HttpStatusSLODefinition{
				Expected:        routeSLO.Status.Expected,
				BreachThreshold: breachThreshold,
				WindowDuration:  time.Duration(routeSLO.Status.WindowDuration),
			},
		}
	}

	return result, nil
}

func parsePercentileDefinitions(percentiles []PercentileDefinition) ([]servicelevels.PercentileDefinition, error) {
	result := make([]servicelevels.PercentileDefinition, len(percentiles))

	for i, percentile := range percentiles {
		percentileValue, parseErr := servicelevels.ParsePercentile(percentile.Percentile)
		if parseErr != nil {
			return nil, parseErr
		}
		result[i] = servicelevels.PercentileDefinition{
			Percentile: percentileValue,
			Threshold:  servicelevels.LatencyMs(percentile.ThresholdMs),
			Name:       percentile.Name,
		}
	}
	return result, nil
}
