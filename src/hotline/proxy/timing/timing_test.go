package timing_test

import (
	. "github.com/onsi/ginkgo/v2"
)

// Phase timings, measured with net/http/httptrace and an injected clock.
// All values are in seconds. What each one is measured from is Q10.
var _ = Describe("Measuring each phase of a call", func() {
	Context("normal cases", func() {
		PIt("records hotline.timing.dns, connect, tls, first_byte and total for a new https connection")
		PIt("records no tls timing for a plain http call")
		PIt("records no dns timing when the vendor host is an IP address")
		PIt("records the vendor IP used as network.peer.address")
		PIt("records the name looked up as dns.question.name")
		PIt("records the vendor's TLS version as tls.protocol.version")
		PIt("records when the vendor's certificate expires as tls.server.not_after")
		PIt("records the HTTP version used to the vendor as network.protocol.version")
		PIt("ends total when the last byte of the reply has gone to the app")
		PIt("records the proxy's own added time as hotline.timing.proxy (Q10)")
		PIt("reads time only from the injected clock, so tests need no real waiting")
	})

	Context("reused connections", func() {
		PIt("sets hotline.connection.reused to true and records no dns, connect or tls timing on a reused connection")
		PIt("sets hotline.connection.reused to false on a new connection")
		PIt("still records network.peer.address on a reused connection")
	})

	Context("failure cases", func() {
		PIt("records the dns timing up to the failure and the total when DNS fails")
		PIt("records dns and connect timings and the total when the TLS handshake fails")
		PIt("records no first_byte timing when the vendor never answered")
		PIt("records phases that happened in parallel, such as a client trying both IPv4 and IPv6, without counting them twice")
	})
})
