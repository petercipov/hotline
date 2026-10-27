package export_test

import (
	. "github.com/onsi/ginkgo/v2"
)

// OpenTelemetry output over OTLP. Standard names come from the semconv Go
// package; Hotline-only fields use the hotline. prefix. Tests use a fake OTLP
// receiver running locally.
var _ = Describe("Sending OpenTelemetry data", func() {
	Context("resource", func() {
		PIt("sets service.name to hotline-proxy on every signal")
		PIt("sets service.version and service.instance.id on every signal")
	})

	Context("request records as logs", func() {
		PIt("sends one OTLP log record per call with all of the record's attributes")
		PIt("uses the semconv constants for every standard name, e.g. http.request.method and url.full")
		PIt("uses the exact hotline. names from the spec for every other field")
		PIt("sends numbers as numbers, true and false as booleans and header values as lists of strings")
	})

	Context("the hotline.requests counter", func() {
		PIt("adds one per call, with unit {request}")
		PIt("carries hotline.integration.id, url.template, http.request.method, http.response.status_code and error.type")
		PIt("leaves url.template out for not-passed and unknown-ID calls")
		PIt("sends cumulative totals by default")
		PIt("sends delta totals to an exporter that sets temporality to delta")
		PIt("sends no latency histogram")
	})

	Context("several destinations", func() {
		PIt("sends the same records and counts to every exporter")
		PIt("keeps sending to the other exporters when one is down")
		PIt("keeps a queue per exporter holding 10 000 records by default")
		PIt("drops the oldest record when a queue is full and counts how many were dropped (Q13)")
		PIt("never blocks or slows a call when a queue is full")
		PIt("sends what is queued before the proxy shuts down, within a time limit")
	})

	Context("left for later slices", func() {
		PIt("sends no trace spans in slice 1")
		PIt("sends no proxy health data to the telemetry exporter in slice 1 (Q13)")
	})
})
