package ingress_test

import (
	. "github.com/onsi/ginkgo/v2"
)

// The ingress finder: which ingress a call used, who sent it and which
// settings apply. Slice 1 builds only the forward proxy ingress.
var _ = Describe("Finding the ingress", func() {
	Context("given a request on the proxy port", func() {
		When("the request line holds a full https address", func() {
			PIt("then it is the forward proxy ingress")
		})
		When("the request line holds a full http address", func() {
			PIt("then it is the forward proxy ingress")
		})
		When("the request line and the Host header name different hosts", func() {
			PIt("then the vendor host and port come from the request line")
		})
		When("the address has no port", func() {
			PIt("then the port is 443 for https and 80 for http")
		})
		When("the scheme and host are in upper case", func() {
			PIt("then they are accepted the same as lower case")
		})
	})

	Context("given slice 1, where only the forward proxy ingress exists", func() {
		When("the path starts with /https://", func() {
			PIt("then it is not the forward proxy ingress; that ingress comes in a later slice (Q1)")
		})
		When("the request has a plain path and a Host header", func() {
			PIt("then it is not the forward proxy ingress; that ingress comes in a later slice (Q1)")
		})
		When("the request is CONNECT", func() {
			PIt("then the proxy answers with an error and a Proxy-Status header and sends nothing to the vendor (Q1)")
		})
		When("the full address has a scheme other than http or https", func() {
			PIt("then the proxy refuses it and sends nothing to the vendor")
		})
		When("the full address has no host", func() {
			PIt("then the proxy refuses it and sends nothing to the vendor")
		})
	})
})

var _ = Describe("Reading the integration ID", func() {
	Context("given integration talon-shop-a with an api for api.talon.one", func() {
		When("Proxy-Authorization is Basic talon-shop-a:x", func() {
			PIt("then the ID is talon-shop-a and the password is ignored")
			PIt("then the api for api.talon.one is picked, with its spec, TLS origination setting and vendorTimeout")
		})
		When("the password is empty, as in talon-shop-a:", func() {
			PIt("then the ID is talon-shop-a")
		})
		When("the word Basic is in any upper or lower case", func() {
			PIt("then the ID is still read")
		})
		When("the call goes to a host in none of its apis", func() {
			PIt("then the not-passed settings apply and the record keeps the ID talon-shop-a")
		})
	})

	Context("given a call without a known ID", func() {
		When("Proxy-Authorization is missing", func() {
			PIt("then the not-passed settings apply and the record says not-passed")
		})
		When("the ID is not in the config", func() {
			PIt("then the not-passed settings apply and the record keeps the ID as sent")
		})
		When("the user name is empty", func() {
			PIt("then the not-passed settings apply (Q2)")
		})
	})

	Context("given a broken Proxy-Authorization header (Q2)", func() {
		When("the scheme is not Basic, such as Bearer", func() {
			PIt("then it is treated like a missing header")
		})
		When("the value is not valid base64", func() {
			PIt("then it is treated like a missing header")
		})
		When("the decoded value has no colon", func() {
			PIt("then it is treated like a missing header")
		})
		When("there are two Proxy-Authorization headers", func() {
			PIt("then they are treated like a missing header")
		})
		When("anything is logged or reported", func() {
			PIt("then the header's value never appears")
		})
	})
})
