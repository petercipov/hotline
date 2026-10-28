package forward_test

import (
	. "github.com/onsi/ginkgo/v2"
)

// The forwarder: makes the call to the vendor and hands the reply back.
// Tests use local vendors from net/http/httptest, never the real internet.
var _ = Describe("Forwarding a call to the vendor", func() {
	Context("given an api with TLS origination on, the default", func() {
		When("the request line says https", func() {
			PIt("then the proxy calls the vendor over TLS")
		})
		When("the request line says http", func() {
			PIt("then the proxy calls the vendor over TLS")
		})
		When("the request line says http with no port", func() {
			PIt("then the proxy uses port 443 (Q4)")
		})
		When("the request line says http with an explicit port", func() {
			PIt("then the proxy keeps that port (Q4)")
		})
	})

	Context("given an api with tlsOrigination set to false", func() {
		When("the request line says http", func() {
			PIt("then the proxy calls the vendor over plain http")
		})
	})

	Context("given a not-passed call with the built-in settings", func() {
		When("the request line says http", func() {
			PIt("then the call goes out as plain http")
		})
		When("the request line says https", func() {
			PIt("then the call goes out as https (Q5)")
		})
	})

	Context("given a not-passed entry with tlsOrigination set to true", func() {
		When("the request line says http", func() {
			PIt("then the proxy calls the vendor over TLS")
		})
	})

	Context("given a call from an app", func() {
		When("the proxy forwards it", func() {
			PIt("then the method, path, query and body reach the vendor unchanged")
			PIt("then Proxy-Authorization is removed")
			PIt("then Authorization and every other end-to-end header are kept")
			PIt("then the headers for one hop only, such as Connection and Proxy-Connection, are removed as HTTP requires")
			PIt("then Host is set to the vendor host from the request line")
			PIt("then no X-Forwarded-For or Via header is added (Q6)")
		})
		When("the request body is large", func() {
			PIt("then it streams to the vendor without waiting for all of it")
		})
		When("the app talks HTTP/1.1", func() {
			PIt("then the proxy answers in HTTP/1.1")
		})
		When("the call fails", func() {
			PIt("then the proxy never retries it")
		})
	})

	Context("given a vendor reply", func() {
		When("it reaches the proxy", func() {
			PIt("then the status, headers and body go back to the app unchanged")
		})
		When("it is an error such as 400, 429 or 503", func() {
			PIt("then it goes back as it is, without a Proxy-Status header")
		})
		When("it is large", func() {
			PIt("then it streams back without waiting for all of it")
		})
	})

	Context("given a vendor that offers HTTP/2", func() {
		When("the proxy calls it", func() {
			PIt("then the proxy uses HTTP/2")
		})
	})

	Context("given a vendor that only offers HTTP/1.1", func() {
		When("the proxy calls it", func() {
			PIt("then the proxy uses HTTP/1.1")
		})
	})

	Context("given a vendor certificate", func() {
		When("its issuer is in the system trust store", func() {
			PIt("then the call goes through")
		})
		When("it is self-signed", func() {
			PIt("then the app gets 502 with Proxy-Status: hotline; error=tls_certificate_error")
		})
		When("it has expired", func() {
			PIt("then the app gets 502 with tls_certificate_error")
		})
		When("it is for another host name", func() {
			PIt("then the app gets 502 with tls_certificate_error")
		})
		When("anyone looks for a setting to turn the check off", func() {
			PIt("then there is none")
		})
	})

	Context("given a call the proxy cannot complete", func() {
		When("the vendor host does not exist", func() {
			PIt("then the app gets 502 with Proxy-Status: hotline; error=dns_error")
		})
		When("the DNS lookup takes too long", func() {
			PIt("then the app gets 504 with Proxy-Status: hotline; error=dns_timeout")
		})
		When("the vendor refuses the connection", func() {
			PIt("then the app gets 502 with Proxy-Status: hotline; error=connection_refused")
		})
		When("connecting takes too long", func() {
			PIt("then the app gets 504 with Proxy-Status: hotline; error=connection_timeout")
		})
		When("the TLS handshake fails for a reason other than the certificate", func() {
			PIt("then the app gets 502 with Proxy-Status: hotline; error=tls_protocol_error")
		})
		When("the vendor takes longer than vendorTimeout", func() {
			PIt("then the app gets 504 with Proxy-Status: hotline; error=http_response_timeout")
		})
		When("the vendor breaks off after the reply has started", func() {
			PIt("then the reply to the app is cut and the record says error.type http_response_incomplete")
		})
		When("any of these happens", func() {
			PIt("then the record's error.type holds the same name as Proxy-Status")
			PIt("then the vendor never gets any part of a call the proxy refused")
		})
	})

	Context("given an app that gives up before the vendor answers", func() {
		When("the app disconnects", func() {
			PIt("then the proxy keeps waiting for the vendor up to vendorTimeout")
			PIt("then the record holds the vendor's real status and timings")
			PIt("then the record says hotline.client.cancelled true")
		})
	})

	Context("given an app that waits for the answer", func() {
		When("the call ends", func() {
			PIt("then the record says hotline.client.cancelled false")
		})
	})
})
