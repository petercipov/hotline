package export_test

import (
	. "github.com/onsi/ginkgo/v2"
)

// OpenTelemetry output over OTLP. Standard names come from the semconv Go
// package; Hotline-only fields use the hotline. prefix. Tests use a fake OTLP
// receiver running locally.
var _ = Describe("Sending OpenTelemetry data", func() {
	Context("given any signal", func() {
		When("it is sent", func() {
			PIt("then its resource has service.name hotline-proxy")
			PIt("then its resource has service.version and service.instance.id")
		})
	})

	Context("given a finished request record", func() {
		When("it is sent", func() {
			PIt("then it is one OTLP log record with all of the record's attributes")
			PIt("then every standard name comes from the semconv constants, such as http.request.method and url.full")
			PIt("then every other field has the exact hotline. name from the spec")
			PIt("then numbers are numbers, true and false are booleans and header values are lists of strings")
		})
	})

	Context("given the hotline.requests counter", func() {
		When("a call ends", func() {
			PIt("then the counter goes up by one, with unit {request}")
			PIt("then it carries hotline.integration.id, url.template, http.request.method, http.response.status_code and error.type")
		})
		When("the call used the not-passed settings", func() {
			PIt("then url.template is left out")
		})
		When("the exporter does not set temporality", func() {
			PIt("then the totals are cumulative")
		})
		When("the exporter sets temporality to delta", func() {
			PIt("then the totals are delta")
		})
		When("metrics are sent", func() {
			PIt("then there is no latency histogram")
		})
	})

	Context("given several exporters", func() {
		When("records and counts are sent", func() {
			PIt("then every exporter gets the same data")
		})
		When("one exporter is down", func() {
			PIt("then the others keep receiving")
		})
	})

	Context("given an exporter queue", func() {
		When("nothing is set", func() {
			PIt("then it holds 10 000 records")
		})
		When("it is full", func() {
			PIt("then the oldest record is dropped and counted (Q13)")
			PIt("then no call is blocked or slowed")
		})
		When("the proxy shuts down", func() {
			PIt("then what is queued is sent first, within a time limit")
		})
	})

	Context("given slice 1, before traces and health data exist", func() {
		When("the proxy runs", func() {
			PIt("then it sends no trace spans")
			PIt("then it sends no proxy health data to the telemetry exporter (Q13)")
		})
	})
})
