package setup_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cucumber/godog"
	"github.com/stretchr/testify/assert"
	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	"google.golang.org/protobuf/proto"
)

type fakeCollector struct {
	metrics []ExpectedMetric
	sync    sync.Mutex
}

func (c *fakeCollector) ServeHTTP(writer http.ResponseWriter, req *http.Request) {
	bodyBytes, bodyReadErr := uncompressedGzip(req.Body)
	if bodyReadErr != nil {
		writer.WriteHeader(http.StatusBadRequest)
		return
	}
	defer func() {
		_ = req.Body.Close()
	}()

	message := &colmetricspb.ExportMetricsServiceRequest{}
	unmarshalErr := proto.Unmarshal(bodyBytes, message)
	if unmarshalErr != nil {
		writer.WriteHeader(http.StatusBadRequest)
		return
	}

	var received []ExpectedMetric
	for _, resource := range message.ResourceMetrics {
		for _, scopeMetrics := range resource.ScopeMetrics {
			for _, metric := range scopeMetrics.Metrics {
				g, isGauge := metric.Data.(*metricspb.Metric_Gauge)
				if isGauge {
					for _, dp := range g.Gauge.DataPoints {
						val := dp.Value.(*metricspb.NumberDataPoint_AsDouble)
						var atts KeyVals
						for _, att := range dp.Attributes {

							_, isBool := att.GetValue().Value.(*commonpb.AnyValue_BoolValue)
							if isBool {
								atts = append(atts, KeyVal{
									Key:   att.Key,
									Value: fmt.Sprintf("%t", att.Value.GetBoolValue()),
								})
							} else {
								atts = append(atts, KeyVal{
									Key:   att.Key,
									Value: att.Value.GetStringValue(),
								})
							}
						}

						received = append(received, ExpectedMetric{
							Name:      metric.Name,
							Unit:      metric.Unit,
							Type:      "Gauge",
							Value:     roundTo(val.AsDouble, 3),
							Timestamp: time.Unix(0, int64(dp.TimeUnixNano)).UTC().Format(time.RFC3339),
							KeyVals:   atts.Sorted(),
						})
					}
					continue
				}

				s, isSum := metric.Data.(*metricspb.Metric_Sum)
				if isSum {
					for _, dp := range s.Sum.DataPoints {
						val := dp.Value.(*metricspb.NumberDataPoint_AsInt)
						var atts KeyVals
						for _, att := range dp.Attributes {
							_, isBool := att.GetValue().Value.(*commonpb.AnyValue_BoolValue)
							if isBool {
								atts = append(atts, KeyVal{
									Key:   att.Key,
									Value: fmt.Sprintf("%t", att.Value.GetBoolValue()),
								})
							} else {
								atts = append(atts, KeyVal{
									Key:   att.Key,
									Value: att.Value.GetStringValue(),
								})
							}
						}

						received = append(received, ExpectedMetric{
							Name:      metric.Name,
							Unit:      metric.Unit,
							Type:      "Sum",
							Value:     roundTo(float64(val.AsInt), 3),
							Timestamp: time.Unix(0, int64(dp.TimeUnixNano)).UTC().Format(time.RFC3339),
							KeyVals:   atts.Sorted(),
						})
					}
				}
			}
		}
	}

	c.sync.Lock()
	c.metrics = append(c.metrics, received...)
	c.sync.Unlock()

	writer.WriteHeader(http.StatusOK)
}

func (c *fakeCollector) GetMetrics() []ExpectedMetric {
	c.sync.Lock()
	metrics := c.metrics
	c.sync.Unlock()

	return metrics
}

func uncompressedGzip(reader io.Reader) ([]byte, error) {
	decompressor, _ := gzip.NewReader(reader)
	var uncompressed bytes.Buffer
	_, err := io.Copy(&uncompressed, decompressor)
	if err != nil {
		return nil, err
	}
	_ = decompressor.Close()
	return uncompressed.Bytes(), nil
}

var errMetricsNotEqual = errors.New("metrics are not equal")

func (c *fakeCollector) ExpectCollectorMetrics(ctx context.Context, t *testing.T, expectedTable *godog.Table) (context.Context, error) {
	header := make(map[string]int)
	for i, headerCell := range expectedTable.Rows[0].Cells {
		header[headerCell.Value] = i
	}

	var expectedMetrics []ExpectedMetric
	for _, row := range expectedTable.Rows[1:] {
		metricName := row.Cells[header["Name"]].Value
		timestampUTC := row.Cells[header["Timestamp UTC"]].Value
		metricType := row.Cells[header["Type"]].Value
		metricValue := row.Cells[header["Value"]].Value
		metricUnit := row.Cells[header["Unit"]].Value
		metricAttrs := row.Cells[header["Attributes"]].Value

		var kv KeyVals
		for _, keyval := range strings.Split(metricAttrs, ";") {
			if len(keyval) == 0 {
				continue
			}
			split := strings.SplitN(strings.TrimSpace(keyval), ":", 2)
			key := strings.TrimSpace(split[0])
			value := strings.TrimSpace(split[1])

			if len(key) == 0 || len(value) == 0 {
				continue
			}

			kv = append(kv, KeyVal{
				Key:   key,
				Value: value,
			})
		}

		value, _ := strconv.ParseFloat(metricValue, 64)
		expectedMetrics = append(expectedMetrics, ExpectedMetric{
			Name:      metricName,
			Timestamp: timestampUTC,
			Unit:      metricUnit,
			Value:     roundTo(value, 3),
			Type:      metricType,
			KeyVals:   kv.Sorted(),
		})
	}

	count := 0
	for {
		receivedMettrics := c.GetMetrics()
		if assert.ObjectsAreEqual(expectedMetrics, receivedMettrics) {
			return ctx, nil
		}
		count++
		if count < 100 {
			time.Sleep(5 * time.Millisecond)
		} else {
			slices.SortFunc(receivedMettrics, func(a, b ExpectedMetric) int {
				typeCmp := strings.Compare(a.Type, b.Type)
				if typeCmp == 0 {
					nameCmp := strings.Compare(a.Name, b.Name)
					if nameCmp == 0 {
						return strings.Compare(a.Timestamp, b.Timestamp)
					}
					return nameCmp
				}
				return typeCmp
			})

			// for _, metric := range receivedMettrics {
			// 	 fmt.Printf("# %s %s %s %.3f %s %s\n", metric.Timestamp, metric.Name, metric.Type, metric.Value, metric.Unit, metric.KeyVals.String())
			// }

			if !assert.Equal(t, expectedMetrics, receivedMettrics, "Metrics are not equal") {
				return ctx, errMetricsNotEqual
			}
			return ctx, nil
		}
	}
}

type ExpectedMetric struct {
	Name      string
	Timestamp string
	Value     float64
	Unit      string
	Type      string
	KeyVals   KeyVals
}

type KeyVal struct {
	Key   string
	Value string
}

type KeyVals []KeyVal

func (kvs KeyVals) Sorted() KeyVals {
	slices.SortFunc(kvs, func(a, b KeyVal) int {
		keyCmp := strings.Compare(a.Key, b.Key)
		if keyCmp == 0 {
			return strings.Compare(a.Value, b.Value)
		}
		return keyCmp
	})
	return kvs
}

func (kvs KeyVals) String() string {
	var sb strings.Builder
	sb.WriteString("[ ")
	for _, kv := range kvs {
		sb.WriteString(kv.Key)
		sb.WriteString(":")
		sb.WriteString(kv.Value)
		sb.WriteString("; ")
	}
	sb.WriteString("]")
	return sb.String()
}

func roundTo(value float64, decimals uint32) float64 {
	return math.Round(value*math.Pow(10, float64(decimals))) / math.Pow(10, float64(decimals))
}
