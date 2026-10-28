package redact_test

import (
	. "github.com/onsi/ginkgo/v2"
)

// Redaction, driven only by the x-hotline-redact marks in our copy of the
// vendor's spec. A redacted value is replaced with the text REDACTED.
var _ = Describe("Redacting the vendor address for url.full", func() {
	Context("given the Talon.One sample call", func() {
		When("its address is redacted", func() {
			PIt("then url.full is https://api.talon.one/v2/customer_sessions/REDACTED?dry=true")
		})
	})

	Context("given the Stripe sample call", func() {
		When("its address is redacted", func() {
			PIt("then url.full is https://api.stripe.com/v1/payment_intents?limit=10&customer=REDACTED")
		})
	})

	Context("given a path with values", func() {
		When("a value has no mark", func() {
			PIt("then it is replaced with REDACTED")
		})
		When("a value is marked false, such as the Stripe payment intent ID", func() {
			PIt("then it is kept")
		})
		When("a part of the path is fixed", func() {
			PIt("then it is kept")
		})
		When("the path has no route in the spec", func() {
			PIt("then every path segment is redacted (Q3)")
		})
	})

	Context("given a query", func() {
		When("a value is marked false, such as dry or limit", func() {
			PIt("then it is kept")
		})
		When("a value has no mark", func() {
			PIt("then it is replaced with REDACTED and its name is kept")
		})
		When("a parameter is not in the spec at all", func() {
			PIt("then its value is replaced with REDACTED")
		})
		When("several parameters are sent", func() {
			PIt("then their order is kept")
		})
		When("a parameter appears more than once", func() {
			PIt("then each of its values is redacted")
		})
		When("a parameter has no value, such as ?verbose", func() {
			PIt("then it is kept as it is")
		})
		When("a parameter has an empty value, such as ?customer=", func() {
			PIt("then it stays empty instead of saying REDACTED (Q11)")
		})
		When("a value is percent-encoded", func() {
			PIt("then it is redacted as one value")
		})
		When("a parameter is a secret in the spec's security section but marked false", func() {
			PIt("then it is still redacted")
		})
	})

	Context("given an address with extra parts", func() {
		When("it holds a user name and password", func() {
			PIt("then they are never recorded")
		})
		When("it has a fragment after #", func() {
			PIt("then the fragment is dropped")
		})
		When("it is redacted", func() {
			PIt("then the scheme, host and port are kept")
		})
	})
})

var _ = Describe("Choosing headers for the record", func() {
	Context("given headers on the default list", func() {
		When("a request or reply has Content-Type, Content-Length, Date or Retry-After", func() {
			PIt("then they are kept")
		})
		When("a reply has the vendor's request ID, Request-Id or X-Request-Id", func() {
			PIt("then it is kept")
		})
		When("a header name starts with RateLimit-, such as RateLimit-Remaining", func() {
			PIt("then it is kept")
		})
	})

	Context("given headers with marks in the spec", func() {
		When("a header is marked false, such as Stripe-Version", func() {
			PIt("then it is kept")
		})
		When("a header is marked true, such as X-Debug-Token", func() {
			PIt("then its name is kept and its value is REDACTED")
		})
	})

	Context("given a header that is neither marked nor on the default list", func() {
		When("the record is built", func() {
			PIt("then the header is left out, name and all")
		})
	})

	Context("given headers that are kept", func() {
		When("the record is built", func() {
			PIt("then they are named http.request.header.<name> and http.response.header.<name>, in lower case")
		})
		When("a header has several values", func() {
			PIt("then they are recorded as a list of values")
		})
	})

	Context("given secret headers", func() {
		When("Authorization is sent, even if the spec marks it false", func() {
			PIt("then it is never recorded")
		})
		When("Proxy-Authorization is sent", func() {
			PIt("then it is never recorded")
		})
		When("Cookie or Set-Cookie is sent", func() {
			PIt("then it is never recorded")
		})
		When("a header is a secret in the spec's security section but marked false", func() {
			PIt("then its value is always redacted")
		})
		When("a secret header comes in lower case, such as authorization", func() {
			PIt("then it is caught the same way")
		})
	})
})
