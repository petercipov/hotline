package record_test

import (
	. "github.com/onsi/ginkgo/v2"
)

// The request record: one OpenTelemetry log record per call, built after the
// reply has gone back to the app. Field names follow proxy-spec.md section 4.1.
var _ = Describe("Building the request record", func() {
	Context("full record for a known integration", func() {
		PIt("sets event.name to hotline.request")
		PIt("sets hotline.integration.id to the ID read from Proxy-Authorization")
		PIt("sets hotline.ingress to forward_proxy")
		PIt("records http.request.method and http.response.status_code")
		PIt("records the redacted address as url.full")
		PIt("records the scheme actually sent to the vendor as url.scheme, so https when the app sent http and TLS origination was on")
		PIt("records the route template from the spec as url.template")
		PIt("leaves url.template out when the path has no route in the spec (Q3)")
		PIt("records the vendor host and port as server.address and server.port")
		PIt("records the bytes that actually passed as http.request.body.size and http.response.body.size, not the Content-Length header")
		PIt("records the allowed headers chosen by the redaction rules")
		PIt("records the phase timings, network and TLS fields")
		PIt("records hotline.client.cancelled and hotline.connection.reused")
		PIt("records hotline.config.version")
		PIt("records error.type only when the call failed")
		PIt("matches the Talon.One sample record from proxy-example, apart from the schema errors that come in a later slice")
	})

	Context("minimal record for not-passed, unknown IDs and unknown hosts", func() {
		PIt("holds only hotline.integration.id, hotline.ingress, server.address, http.request.method, http.response.status_code, error.type and the hotline.timing fields")
		PIt("records no path, query, headers or body fingerprints")
		PIt("records the ID not-passed when no ID was sent")
		PIt("records the ID as sent when the ID is unknown, so typos stay visible")
		PIt("records the ID as sent when the host is in none of the integration's apis")
	})

	Context("fields left for later slices (Q12)", func() {
		PIt("leaves out hotline.schema.request.errors and hotline.schema.response.errors")
		PIt("leaves out hotline.body.request.hmac_sha256 and hotline.body.response.hmac_sha256")
	})

	Context("keeping secrets and the call safe", func() {
		PIt("never holds the value of Authorization, Proxy-Authorization, a cookie or an API key anywhere in the record")
		PIt("never holds any byte of the request or response body")
		PIt("builds the record only after the reply has gone back to the app")
		PIt("does not slow the call when building the record is slow")
	})
})
