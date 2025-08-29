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

func ParseRoute(latencyDefinition LatencySLODefinition, statusDefinition StatusSLODefinition, route Route) (servicelevels.HttpRouteSLODefinition, error) {
	percentile := statusDefinition.BreachThreshold.Cast()

	defs, defsErr := parsePercentileDefinitions(latencyDefinition.Percentiles)
	if defsErr != nil {
		return servicelevels.HttpRouteSLODefinition{}, defsErr
	}

	return servicelevels.HttpRouteSLODefinition{
		Route: http.Route{
			Method:      optString((*string)(route.Method), ""),
			PathPattern: optString(route.Path, ""),
			Host:        optString(route.Host, ""),
			Port:        int(optInt32(route.Port, 0)),
		},
		Latency: servicelevels.HttpLatencySLODefinition{
			Percentiles:    defs,
			WindowDuration: time.Duration(latencyDefinition.WindowDuration),
		},
		Status: servicelevels.HttpStatusSLODefinition{
			Expected:        convertFromExpected(statusDefinition.Expected),
			BreachThreshold: *percentile,
			WindowDuration:  time.Duration(statusDefinition.WindowDuration),
		},
	}, nil
}

func convertFromExpected(expected []StatusSLODefinitionExpected) []string {
	arr := make([]string, len(expected))
	for i, e := range expected {
		arr[i] = string(e)
	}
	return arr
}

func convertToExpected(expected []string) []StatusSLODefinitionExpected {
	arr := make([]StatusSLODefinitionExpected, len(expected))
	for i, e := range expected {
		arr[i] = StatusSLODefinitionExpected(e)
	}
	return arr
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

func convertRoutes(routes []servicelevels.HttpRouteSLODefinition) []RouteSLODefinition {
	var defs []RouteSLODefinition
	for _, route := range routes {
		method := RouteMethod(route.Route.Method)
		defs = append(defs, RouteSLODefinition{
			Latency: LatencySLODefinition{
				Percentiles:    convertPercentiles(route.Latency.Percentiles),
				WindowDuration: Duration(route.Latency.WindowDuration),
			},
			Status: StatusSLODefinition{
				BreachThreshold: Percentile(route.Status.BreachThreshold),
				Expected:        convertToExpected(route.Status.Expected),
				WindowDuration:  Duration(route.Status.WindowDuration),
			},
			Route: Route{
				Host:   ptrString(route.Route.Host),
				Method: &method,
				Path:   ptrString(route.Route.PathPattern),
				Port:   ptrInt32(int32(route.Route.Port)),
			},
			RouteKey: route.Route.ID(),
		})
	}

	return defs
}

func convertPercentiles(percentiles []servicelevels.PercentileDefinition) []PercentileThreshold {
	var result []PercentileThreshold
	for _, percentile := range percentiles {
		result = append(result, PercentileThreshold{
			BreachLatency: Duration(percentile.Threshold.AsDuration()),
			Percentile:    Percentile(percentile.Percentile),
		})
	}
	return result
}

func optString(val *string, def string) string {
	if val == nil {
		return def
	}
	return *val
}

func optInt32(val *int32, def int32) int32 {
	if val == nil {
		return def
	}
	return *val
}

func ptrString(val string) *string {
	if val == "" {
		return nil
	}
	return &val
}

func ptrInt32(val int32) *int32 {
	if val == 0 {
		return nil
	}
	return &val
}
