package timing_test

import (
	. "github.com/onsi/ginkgo/v2"
)

// Phase timings, measured with net/http/httptrace and an injected clock.
// All values are in seconds. What each one is measured from is Q10.
var _ = Describe("Measuring each phase of a call", func() {
	Context("given a new https connection to the vendor", func() {
		When("the call ends", func() {
			PIt("then the record has hotline.timing.dns, connect, tls, first_byte and total")
			PIt("then the record has the vendor IP as network.peer.address")
			PIt("then the record has the name looked up as dns.question.name")
			PIt("then the record has the vendor's TLS version as tls.protocol.version")
			PIt("then the record has when the vendor's certificate expires as tls.server.not_after")
			PIt("then the record has the HTTP version used to the vendor as network.protocol.version")
			PIt("then the record has the proxy's own added time as hotline.timing.proxy (Q10)")
			PIt("then the record says hotline.connection.reused false")
		})
		When("the last byte of the reply has gone to the app", func() {
			PIt("then total ends at that moment")
		})
	})

	Context("given a plain http call", func() {
		When("the call ends", func() {
			PIt("then there is no tls timing")
		})
	})

	Context("given a vendor host that is an IP address", func() {
		When("the call ends", func() {
			PIt("then there is no dns timing")
		})
	})

	Context("given a connection that is already open", func() {
		When("a call reuses it", func() {
			PIt("then the record says hotline.connection.reused true")
			PIt("then there are no dns, connect or tls timings")
			PIt("then network.peer.address is still recorded")
		})
	})

	Context("given a clock injected through the constructor", func() {
		When("timings are measured", func() {
			PIt("then time is read only from that clock, so tests need no real waiting")
		})
	})

	Context("given a call that fails", func() {
		When("DNS fails", func() {
			PIt("then the record has the dns timing up to the failure and the total")
		})
		When("the TLS handshake fails", func() {
			PIt("then the record has the dns and connect timings and the total")
		})
		When("the vendor never answers", func() {
			PIt("then there is no first_byte timing")
		})
	})

	Context("given a client that tries IPv4 and IPv6 at the same time", func() {
		When("the phases overlap", func() {
			PIt("then nothing is counted twice")
		})
	})
})
