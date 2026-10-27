package forward_test

import (
	. "github.com/onsi/ginkgo/v2"
)

// The forwarder: makes the call to the vendor and hands the reply back.
// Tests use local vendors from net/http/httptest, never the real internet.
var _ = Describe("Forwarding a call to the vendor", func() {
	Context("choosing http or https to the vendor", func() {
		PIt("calls the vendor over TLS when the request line says https")
		PIt("calls the vendor over TLS when the request line says http and TLS origination is on, which is the default")
		PIt("uses port 443 when it turns an http address without a port into https (Q4)")
		PIt("keeps an explicit port when it turns http into https (Q4)")
		PIt("calls the vendor over plain http when the request line says http and the api sets tlsOrigination to false")
		PIt("sends a not-passed call out exactly as sent: http stays http and https stays https (Q5)")
		PIt("turns http into https for not-passed calls when the not-passed entry sets tlsOrigination to true")
	})

	Context("the request sent to the vendor", func() {
		PIt("passes the method, path, query and body on unchanged")
		PIt("removes Proxy-Authorization before calling the vendor")
		PIt("keeps Authorization and every other end-to-end header")
		PIt("removes the headers that only apply to one hop, such as Connection and Proxy-Connection, as HTTP requires")
		PIt("sets Host to the vendor host from the request line")
		PIt("adds no X-Forwarded-For or Via header (Q6)")
		PIt("streams a large request body to the vendor without waiting for all of it")
		PIt("never retries a call that failed")
	})

	Context("the reply sent back to the app", func() {
		PIt("passes the status, headers and body back unchanged")
		PIt("passes vendor errors such as 400, 429 and 503 back as they are, without a Proxy-Status header")
		PIt("streams a large reply back without waiting for all of it")
		PIt("talks HTTP/2 to the vendor when the vendor offers it and HTTP/1.1 otherwise")
		PIt("accepts HTTP/1.1 from the app and answers in HTTP/1.1")
	})

	Context("vendor certificates", func() {
		PIt("accepts a certificate from an issuer in the system trust store")
		PIt("refuses a self-signed certificate with 502 and Proxy-Status: hotline; error=tls_certificate_error")
		PIt("refuses an expired certificate with 502 and tls_certificate_error")
		PIt("refuses a certificate for another host name with 502 and tls_certificate_error")
		PIt("has no setting that turns the certificate check off")
	})

	Context("the proxy's own errors", func() {
		PIt("answers 502 with Proxy-Status: hotline; error=dns_error when the vendor host does not exist")
		PIt("answers 504 with Proxy-Status: hotline; error=dns_timeout when the DNS lookup takes too long")
		PIt("answers 502 with Proxy-Status: hotline; error=connection_refused when the vendor refuses the connection")
		PIt("answers 504 with Proxy-Status: hotline; error=connection_timeout when connecting takes too long")
		PIt("answers 502 with Proxy-Status: hotline; error=tls_protocol_error when the TLS handshake fails for a reason other than the certificate")
		PIt("answers 504 with Proxy-Status: hotline; error=http_response_timeout when the vendor takes longer than vendorTimeout")
		PIt("records error.type as http_response_incomplete when the vendor breaks off after the reply has started, and cuts the reply to the app")
		PIt("uses the same error name in Proxy-Status and in the record's error.type")
		PIt("never sends the vendor any part of a call it refused")
	})

	Context("when the app gives up first", func() {
		PIt("keeps waiting for the vendor up to vendorTimeout after the app disconnects")
		PIt("records the vendor's real status and timings for a call the app gave up on")
		PIt("sets hotline.client.cancelled to true when the app gave up, and to false otherwise")
	})
})
