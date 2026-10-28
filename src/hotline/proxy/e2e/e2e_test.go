package e2e_test

import (
	. "github.com/onsi/ginkgo/v2"
)

// End-to-end checks for slice 1: real clients call the real proxy binary,
// which calls a local TLS test vendor, and a local fake OTLP receiver
// collects what the proxy sends. They reuse the client matrix from the
// research folder (hotline-matrix-test and matrix-clients/go and node).
// Matrix case numbers are those in egress-proxy-options.md. Only the cases
// with a full address in the request line belong to the forward proxy ingress.
var _ = Describe("Calling through the forward proxy ingress", func() {
	Context("given the Go client", func() {
		When("case 1: it sends POST https://api.stripe.com/... over plain HTTP/1.1", func() {
			PIt("then it gets the vendor's answer, which the proxy fetched over TLS")
		})
		When("case 2: it sends POST http://api.stripe.com/... through Transport.Proxy", func() {
			PIt("then the proxy calls the vendor over https")
		})
		When("case 4: it sends the https address over TLS to the proxy", func() {
			PIt("then it gets the vendor's answer (Q14)")
		})
		When("case 5: it sends the http address over TLS to the proxy through Transport.Proxy", func() {
			PIt("then it gets the vendor's answer (Q14)")
		})
	})

	Context("given the Node.js client", func() {
		When("case 1: it sends POST https://api.stripe.com/... with http.request and a full URL as the path", func() {
			PIt("then it gets the vendor's answer, which the proxy fetched over TLS")
		})
		When("case 2: it sends POST http://api.stripe.com/...", func() {
			PIt("then the proxy calls the vendor over https")
		})
		When("case 4: it sends the https address with https.request and servername set to the proxy", func() {
			PIt("then it gets the vendor's answer (Q14)")
		})
		When("case 5: it sends the http address with https.request", func() {
			PIt("then it gets the vendor's answer (Q14)")
		})
	})

	Context("given any of the cases above", func() {
		When("the call ends", func() {
			PIt("then the app gets the test vendor's status, headers and body unchanged")
			PIt("then the test vendor never saw Proxy-Authorization")
			PIt("then the fake OTLP receiver got one hotline.request log record with ingress forward_proxy and the right integration ID")
			PIt("then the fake OTLP receiver saw hotline.requests go up by one")
			PIt("then the record holds the redacted url.full and no secret")
		})
	})

	Context("given a client calling through the proxy", func() {
		When("the vendor's certificate is not trusted", func() {
			PIt("then the app gets 502 with Proxy-Status and the record says error.type tls_certificate_error")
		})
		When("the vendor host does not exist", func() {
			PIt("then the app gets 502 with Proxy-Status and the record says error.type dns_error")
		})
		When("the call has no Proxy-Authorization", func() {
			PIt("then the call still reaches the vendor and gets a minimal record with ID not-passed")
		})
	})
})
