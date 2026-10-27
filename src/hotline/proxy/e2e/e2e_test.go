package e2e_test

import (
	. "github.com/onsi/ginkgo/v2"
)

// End-to-end checks for slice 1: real clients call the real proxy binary,
// which calls a local TLS test vendor, and a local fake OTLP receiver
// collects what the proxy sends. They reuse the client matrix from the
// research folder (hotline-matrix-test and matrix-clients/go and node).
// Matrix case numbers are those in egress-proxy-options.md. Only the cases
// with a full address in the request line belong to the forward proxy door.
var _ = Describe("Calling through the forward proxy door", func() {
	Context("Go client", func() {
		PIt("case 1: sends POST https://api.stripe.com/... over plain HTTP/1.1 and gets the vendor's answer over TLS")
		PIt("case 2: sends POST http://api.stripe.com/... through Transport.Proxy and the proxy turns it into https")
		PIt("case 4: sends the https address over TLS to the proxy (Q14)")
		PIt("case 5: sends the http address over TLS to the proxy through Transport.Proxy (Q14)")
	})

	Context("Node.js client", func() {
		PIt("case 1: sends POST https://api.stripe.com/... with http.request and a full URL as the path")
		PIt("case 2: sends POST http://api.stripe.com/... and the proxy turns it into https")
		PIt("case 4: sends the https address with https.request and servername set to the proxy (Q14)")
		PIt("case 5: sends the http address with https.request (Q14)")
	})

	Context("what every case checks", func() {
		PIt("the app gets the test vendor's status, headers and body unchanged")
		PIt("the test vendor never sees Proxy-Authorization")
		PIt("the fake OTLP receiver gets one hotline.request log record with door forward_proxy and the right integration ID")
		PIt("the fake OTLP receiver gets hotline.requests increased by one")
		PIt("the record holds the redacted url.full and no secret")
	})

	Context("failure cases", func() {
		PIt("a call to a vendor with an untrusted certificate gets 502 with Proxy-Status and a record with error.type tls_certificate_error")
		PIt("a call to a host that does not exist gets 502 with Proxy-Status and a record with error.type dns_error")
		PIt("a call without Proxy-Authorization still reaches the vendor and gets a minimal record with ID not-passed")
	})
})
