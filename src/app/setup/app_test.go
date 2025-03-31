package setup_test

import (
	"app/setup"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"github.com/stretchr/testify/assert"
	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	"google.golang.org/protobuf/proto"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cucumber/godog"
)

type appSut struct {
	cfg         setup.Config
	app         *setup.App
	managedTime *setup.ManualTime

	collectorServer setup.HttpServer

	ingestionClient *IngestionClient
	fakeCollector   *fakeCollector
}

func (a *appSut) otelIngestionIsEnabled() {
	a.cfg.OtelHttpIngestion.Host = "localhost"
}

func (a *appSut) sloReporterIsPointingToCollector() {
	a.cfg.OtelHttpReporter.Host = a.collectorServer.Host()
}

func (a *appSut) sendTraffic(ctx context.Context, integrationID string) (context.Context, error) {
	now := a.managedTime.Now()
	statusCode, sendErr := a.ingestionClient.SendSomeTraffic(now, integrationID)
	if sendErr != nil {
		return ctx, sendErr
	}

	if statusCode != http.StatusCreated {
		return ctx, errors.New(fmt.Sprint("unexpected status code: ", statusCode))
	}

	nowString := now.UTC().String()
	return godog.Attach(ctx, godog.Attachment{
		FileName:  "current.time: " + nowString,
		MediaType: "text/plain",
	}), nil
}

func (a *appSut) advanceTime(ctx context.Context, seconds int) (context.Context, error) {
	a.managedTime.Advance(time.Duration(seconds) * time.Second)
	nowString := a.managedTime.Now().UTC().String()

	time.Sleep(1000 * time.Millisecond)
	return godog.Attach(ctx, godog.Attachment{
		FileName:  "current.time: " + nowString,
		MediaType: "text/plain",
	}), nil
}

func (a *appSut) runHotline() error {
	app, appErr := setup.NewApp(&a.cfg, a.managedTime, NewTestHttpServer)
	if appErr != nil {
		return appErr
	}
	a.app = app
	a.app.Start()

	a.ingestionClient = &IngestionClient{
		URL: a.app.GetIngestionUrl(),
	}
	return nil
}

func (a *appSut) sloMetricsAreReceivedInCollector(ctx context.Context, metrics *godog.Table) (context.Context, error) {
	header := make(map[string]int)
	for i, headerCell := range metrics.Rows[0].Cells {
		header[headerCell.Value] = i
	}

	var expectedMetrics []ExpectedMetric
	for _, row := range metrics.Rows[1:] {
		metricName := row.Cells[header["Metric Name"]].Value
		timestampUTC := row.Cells[header["Timestamp UTC"]].Value
		metricType := row.Cells[header["Metric Type"]].Value
		metricValue := row.Cells[header["Metric Value"]].Value
		metricUnit := row.Cells[header["Metric Unit"]].Value
		metricAttrs := row.Cells[header["Metric Attributes"]].Value

		var kv []KeyVal
		for _, keyval := range strings.Split(metricAttrs, ";") {
			split := strings.Split(strings.TrimSpace(keyval), ":")
			key := strings.TrimSpace(split[0])
			value := strings.TrimSpace(split[1])
			kv = append(kv, KeyVal{
				Key:   key,
				Value: value,
			})
		}

		slices.SortFunc(kv, func(a, b KeyVal) int {
			keyCmp := strings.Compare(a.Key, b.Key)
			if keyCmp == 0 {
				return strings.Compare(a.Value, b.Value)
			}
			return keyCmp
		})
		value, _ := strconv.ParseFloat(metricValue, 64)
		expectedMetrics = append(expectedMetrics, ExpectedMetric{
			Name:      metricName,
			Timestamp: timestampUTC,
			Unit:      metricUnit,
			Value:     roundTo(value, 3),
			Type:      metricType,
			KeyVals:   kv,
		})
	}

	count := 0
	for {
		receivedMettrics := a.fakeCollector.GetMetrics()
		if assert.ObjectsAreEqual(expectedMetrics, receivedMettrics) {
			return ctx, nil
		}
		count++
		if count < 100 {
			time.Sleep(5 * time.Millisecond)
		} else {
			e := &errorT{}
			assert.Equal(e, expectedMetrics, receivedMettrics, "Metrics are not equal")
			return ctx, e.err
		}
	}
}

type errorT struct {
	err error
}

func (e *errorT) Errorf(format string, args ...interface{}) {
	e.err = fmt.Errorf(format, args...)
}

type ExpectedMetric struct {
	Name      string
	Timestamp string
	Value     float64
	Unit      string
	Type      string
	KeyVals   []KeyVal
}

type KeyVal struct {
	Key   string
	Value string
}

func (a *appSut) Close() error {
	collectorErr := a.collectorServer.Close()
	if collectorErr != nil {
		return collectorErr
	}
	appStopErr := a.app.Stop()
	if appStopErr != nil {
		return appStopErr
	}
	return nil
}

func TestApp(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: func(sctx *godog.ScenarioContext) {
			collector := &fakeCollector{}
			sut := &appSut{
				fakeCollector:   collector,
				managedTime:     setup.NewManualTime(parseTime("2025-02-22T12:02:10Z")),
				collectorServer: NewTestHttpServer("", collector),
			}

			sctx.After(func(ctx context.Context, sc *godog.Scenario, err error) (context.Context, error) {
				sut.Close()
				return ctx, err
			})

			sctx.Given("OTEL ingestion is enabled", sut.otelIngestionIsEnabled)
			sctx.Given("slo reporter is pointing to collector", sut.sloReporterIsPointingToCollector)
			sctx.Given("hotline is running", sut.runHotline)

			sctx.When(`traffic is sent to OTEL ingestion for integration ID "([^"]*)"`, sut.sendTraffic)
			sctx.When("advance time by (\\d+)s", sut.advanceTime)

			sctx.Then("slo metrics are received in collector", sut.sloMetricsAreReceivedInCollector)
		},
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"features"},
			TestingT: t,
		},
	}
	suite.Run()
}

type TestHttpServer struct {
	server *httptest.Server
}

func (t *TestHttpServer) Host() string {
	u, _ := url.Parse(t.server.URL)
	return u.Host
}

func (t *TestHttpServer) Start() {
	slog.Info("Starting test server", slog.Any("server", t.server.URL))
	if len(t.server.URL) == 0 {
		t.server.Start()
	}
}

func (t *TestHttpServer) Close() error {
	slog.Info("Closing test server", slog.Any("server", t.server.URL))
	t.server.Close()
	return nil
}

func NewTestHttpServer(_ string, handler http.Handler) setup.HttpServer {
	return &TestHttpServer{
		server: httptest.NewServer(handler),
	}
}

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
	defer req.Body.Close()

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
				g := metric.Data.(*metricspb.Metric_Gauge)
				for _, dp := range g.Gauge.DataPoints {
					val := dp.Value.(*metricspb.NumberDataPoint_AsDouble)
					var atts []KeyVal
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
					slices.SortFunc(atts, func(a, b KeyVal) int {
						keyCmp := strings.Compare(a.Key, b.Key)
						if keyCmp == 0 {
							return strings.Compare(a.Value, b.Value)
						}
						return keyCmp
					})

					received = append(received, ExpectedMetric{
						Name:      metric.Name,
						Unit:      metric.Unit,
						Type:      "Gauge",
						Value:     roundTo(val.AsDouble, 3),
						Timestamp: time.Unix(0, int64(dp.TimeUnixNano)).UTC().Format(time.RFC3339),
						KeyVals:   atts,
					})
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
	m := c.metrics
	c.sync.Unlock()

	return m
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

func parseTime(nowString string) time.Time {
	now, _ := time.Parse(time.RFC3339, nowString)
	return now
}

func roundTo(value float64, decimals uint32) float64 {
	return math.Round(value*math.Pow(10, float64(decimals))) / math.Pow(10, float64(decimals))
}
