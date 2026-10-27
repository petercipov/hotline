package redact_test

import (
	. "github.com/onsi/ginkgo/v2"
)

// Redaction, driven only by the x-hotline-redact marks in our copy of the
// vendor's spec. A redacted value is replaced with the text REDACTED.
var _ = Describe("Redacting the vendor address for url.full", func() {
	Context("normal cases", func() {
		PIt("replaces an unmarked path value with REDACTED, e.g. /v2/customer_sessions/REDACTED")
		PIt("keeps a path value marked x-hotline-redact false, e.g. the Stripe payment intent ID")
		PIt("keeps the fixed parts of the path")
		PIt("keeps a query value marked false, e.g. dry=true and limit=10")
		PIt("replaces an unmarked query value with REDACTED and keeps its name, e.g. customer=REDACTED")
		PIt("replaces the value of a query parameter the spec does not list with REDACTED")
		PIt("keeps the scheme, host and port")
		PIt("matches the Talon.One sample: https://api.talon.one/v2/customer_sessions/REDACTED?dry=true")
		PIt("matches the Stripe sample: https://api.stripe.com/v1/payment_intents?limit=10&customer=REDACTED")
	})

	Context("edge cases", func() {
		PIt("keeps the order of query parameters as sent")
		PIt("redacts each value of a query parameter that appears more than once")
		PIt("keeps a query parameter with no value as it is, e.g. ?verbose")
		PIt("redacts a percent-encoded value as one value")
		PIt("keeps an empty query value empty instead of writing REDACTED (Q11)")
		PIt("always redacts a query secret named in the spec's security section, even when marked false")
		PIt("never records a user name or password written inside the address")
		PIt("drops a fragment after # from the address")
		PIt("redacts every path segment when the path has no route in the spec (Q3)")
	})
})

var _ = Describe("Choosing headers for the record", func() {
	Context("normal cases", func() {
		PIt("keeps Content-Type, Content-Length, Date and Retry-After on requests and replies")
		PIt("keeps the vendor's request ID, Request-Id or X-Request-Id")
		PIt("keeps every header whose name starts with RateLimit-, e.g. RateLimit-Remaining")
		PIt("keeps a header marked x-hotline-redact false, e.g. Stripe-Version")
		PIt("keeps the name but writes REDACTED for a header marked true, e.g. X-Debug-Token")
		PIt("leaves out an unmarked header that is not on the default list, name and all")
		PIt("names attributes http.request.header.<name> and http.response.header.<name> with the name in lower case")
		PIt("records a header with several values as a list of values")
	})

	Context("secrets", func() {
		PIt("never records Authorization, even when the spec marks it false")
		PIt("never records Proxy-Authorization")
		PIt("never records Cookie or Set-Cookie")
		PIt("always redacts a header named as a secret in the spec's security section, even when marked false")
		PIt("matches header names without regard to upper or lower case, so authorization is caught too")
	})
})
