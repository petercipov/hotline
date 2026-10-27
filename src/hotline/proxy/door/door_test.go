package door_test

import (
	. "github.com/onsi/ginkgo/v2"
)

// The door finder: which door a call used, who sent it and which settings
// apply. Slice 1 builds only the forward proxy door.
var _ = Describe("Finding the door", func() {
	Context("forward proxy door", func() {
		PIt("takes a full https address in the request line as the forward proxy door")
		PIt("takes a full http address in the request line as the forward proxy door")
		PIt("takes the vendor host and port from the request line, not from the Host header")
		PIt("uses port 443 for an https address and port 80 for an http address when none is given")
		PIt("accepts the scheme and host in upper or lower case")
	})

	Context("other requests on the proxy port in slice 1", func() {
		PIt("does not take a path starting with /https:// as the forward proxy door; that door comes in a later slice (Q1)")
		PIt("does not take a plain path with a Host header as the forward proxy door; that door comes in a later slice (Q1)")
		PIt("refuses a CONNECT request with an error answer and a Proxy-Status header, sending nothing to the vendor (Q1)")
		PIt("refuses a full address with any scheme other than http or https, sending nothing to the vendor")
		PIt("refuses a full address with no host, sending nothing to the vendor")
	})
})

var _ = Describe("Reading the integration ID", func() {
	Context("normal cases", func() {
		PIt("reads the ID from the user name of Proxy-Authorization: Basic, e.g. talon-shop-a from talon-shop-a:x")
		PIt("ignores the password")
		PIt("accepts an empty password, e.g. talon-shop-a:")
		PIt("accepts the word Basic in any upper or lower case")
		PIt("picks the integration's api whose hosts list the vendor host")
		PIt("picks that api's spec, TLS origination setting and vendorTimeout")
	})

	Context("calls without a known ID", func() {
		PIt("uses the not-passed settings and records the ID not-passed when Proxy-Authorization is missing")
		PIt("uses the not-passed settings but records the ID as sent when the ID is not in the config")
		PIt("uses the not-passed settings but records the ID as sent when the vendor host is in none of the integration's apis")
		PIt("uses the not-passed settings when the user name is empty (Q2)")
	})

	Context("broken Proxy-Authorization headers (Q2)", func() {
		PIt("treats a scheme other than Basic, e.g. Bearer, like a missing header")
		PIt("treats a value that is not valid base64 like a missing header")
		PIt("treats a decoded value with no colon like a missing header")
		PIt("treats two Proxy-Authorization headers like a missing header")
		PIt("never puts the header's value into a log or error")
	})
})
