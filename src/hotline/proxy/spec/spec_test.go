package spec_test

import (
	. "github.com/onsi/ginkgo/v2"
)

// Reading our copy of a vendor's OpenAPI spec: route templates, the
// x-hotline-redact marks and the secrets named in the security section.
// Body schemas are read in a later slice, together with the JSON checks.
var _ = Describe("Reading the vendor spec", func() {
	Context("given the Talon.One spec", func() {
		When("the call is PUT /v2/customer_sessions/sess-8812", func() {
			PIt("then the route template is /v2/customer_sessions/{customerSessionId}")
		})
		When("the call is GET /v2/customer_sessions/sess-8812, a method the spec does not list", func() {
			PIt("then there is no route")
		})
		When("the call's path is not in the spec", func() {
			PIt("then there is no route (Q3)")
		})
		When("the path has more segments than the template", func() {
			PIt("then one path parameter does not swallow several segments and there is no route")
		})
		When("the path value is percent-encoded", func() {
			PIt("then it matches as one parameter")
		})
	})

	Context("given the Stripe spec", func() {
		When("the call is GET /v1/payment_intents", func() {
			PIt("then the route template is /v1/payment_intents")
		})
	})

	Context("given a spec with a fixed route and a templated route that both match", func() {
		When("the call matches both", func() {
			PIt("then the fixed route wins, as OpenAPI says")
		})
	})

	Context("given a spec whose route ends with a slash", func() {
		When("the call has no trailing slash", func() {
			PIt("then it is matched the way the spec writes the route (Q9)")
		})
	})

	Context("given a spec whose servers address has a base path, such as https://api.x.com/v1", func() {
		When("a call comes in", func() {
			PIt("then the base path is taken into account when matching (Q9)")
		})
	})

	Context("given parameters with x-hotline-redact marks", func() {
		When("a path parameter is marked false", func() {
			PIt("then its value is to be kept")
		})
		When("a query parameter is marked false", func() {
			PIt("then its value is to be kept")
		})
		When("a header parameter is marked true", func() {
			PIt("then its name is to be kept and its value redacted")
		})
		When("a parameter has no mark", func() {
			PIt("then it counts as not marked")
		})
		When("a parameter is listed for the whole route and again for one method", func() {
			PIt("then the method's own entry wins")
		})
		When("the spec declares response headers with marks", func() {
			PIt("then those marks are read too")
		})
		When("a header name differs only in upper or lower case", func() {
			PIt("then it matches the same parameter")
		})
	})

	Context("given a security section", func() {
		When("it has an http bearer or basic scheme", func() {
			PIt("then Authorization is a secret")
		})
		When("it has an apiKey scheme in a header, such as Talon.One's Authorization", func() {
			PIt("then that header is a secret")
		})
		When("it has an apiKey scheme in the query", func() {
			PIt("then that query parameter is a secret")
		})
		When("it has an apiKey scheme in a cookie", func() {
			PIt("then that cookie is a secret")
		})
		When("one operation has its own security section", func() {
			PIt("then its secrets are added to the global ones")
		})
	})
})
