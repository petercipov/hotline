package record_test

import (
	. "github.com/onsi/ginkgo/v2"
)

// The request record: one OpenTelemetry log record per call, built after the
// reply has gone back to the app. Field names follow proxy-spec.md section 4.1.
var _ = Describe("Building the request record", func() {
	Context("given a call from a known integration to one of its apis", func() {
		When("the call ends", func() {
			PIt("then event.name is hotline.request")
			PIt("then hotline.integration.id is the ID read from Proxy-Authorization")
			PIt("then hotline.ingress is forward_proxy")
			PIt("then http.request.method and http.response.status_code are recorded")
			PIt("then url.full holds the redacted address")
			PIt("then url.template holds the route template from the spec")
			PIt("then server.address and server.port hold the vendor host and port")
			PIt("then the allowed headers chosen by the redaction rules are recorded")
			PIt("then the phase timings, network and TLS fields are recorded")
			PIt("then hotline.client.cancelled, hotline.connection.reused and hotline.config.version are recorded")
		})
		When("the app sent http and TLS origination was on", func() {
			PIt("then url.scheme is https, the scheme actually sent to the vendor")
		})
		When("the path has no route in the spec", func() {
			PIt("then url.template is left out (Q3)")
		})
		When("the Content-Length header differs from the bytes that passed", func() {
			PIt("then http.request.body.size and http.response.body.size hold the bytes that passed")
		})
		When("the call succeeded", func() {
			PIt("then there is no error.type")
		})
		When("the call failed", func() {
			PIt("then error.type holds the error name")
		})
	})

	Context("given the Talon.One sample call", func() {
		When("the record is built", func() {
			PIt("then it matches the sample record in proxy-example, apart from the schema errors that come in a later slice")
		})
	})

	Context("given a call that uses the not-passed settings", func() {
		When("the record is built", func() {
			PIt("then it holds only hotline.integration.id, hotline.ingress, server.address, http.request.method, http.response.status_code, error.type and the hotline.timing fields")
			PIt("then it has no path, query, headers or body fingerprints")
		})
		When("no ID was sent", func() {
			PIt("then the ID is not-passed")
		})
		When("the ID was unknown", func() {
			PIt("then the ID is kept as sent, so typos stay visible")
		})
		When("the host was in none of the integration's apis", func() {
			PIt("then the ID is kept as sent")
		})
	})

	Context("given slice 1, before body checks and fingerprints exist (Q12)", func() {
		When("the record is built", func() {
			PIt("then hotline.schema.request.errors and hotline.schema.response.errors are left out")
			PIt("then hotline.body.request.hmac_sha256 and hotline.body.response.hmac_sha256 are left out")
		})
	})

	Context("given a call that carries secrets and bodies", func() {
		When("the record is built", func() {
			PIt("then no value of Authorization, Proxy-Authorization, a cookie or an API key appears anywhere in it")
			PIt("then no byte of the request or response body appears in it")
		})
	})

	Context("given a reply on its way to the app", func() {
		When("the record is built", func() {
			PIt("then building starts only after the reply has gone back")
		})
		When("building the record is slow", func() {
			PIt("then the call is not slowed")
		})
	})
})
