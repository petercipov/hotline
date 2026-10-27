package spec_test

import (
	. "github.com/onsi/ginkgo/v2"
)

// Reading our copy of a vendor's OpenAPI spec: route templates, the
// x-hotline-redact marks and the secrets named in the security section.
// Body schemas are read in a later slice, together with the JSON checks.
var _ = Describe("Reading the vendor spec", func() {
	Context("finding the route", func() {
		PIt("finds the route template for a path, e.g. /v2/customer_sessions/sess-8812 gives /v2/customer_sessions/{customerSessionId}")
		PIt("finds a route with no parameters, e.g. /v1/payment_intents")
		PIt("prefers a fixed route over a templated one when both match, as OpenAPI says")
		PIt("matches the method too: a GET on a route the spec only has for PUT counts as no route")
		PIt("says there is no route when the path is not in the spec (Q3)")
		PIt("does not let one path parameter swallow several path segments")
		PIt("matches a percent-encoded path value as one parameter")
		PIt("handles a trailing slash the way the spec writes the route (Q9)")
		PIt("takes a base path from the spec's servers address into account (Q9)")
	})

	Context("reading the marks", func() {
		PIt("reads x-hotline-redact false on a path parameter")
		PIt("reads x-hotline-redact false on a query parameter")
		PIt("reads x-hotline-redact true and false on a header parameter")
		PIt("treats a parameter with no mark as not marked")
		PIt("reads parameters listed for the whole route as well as for one method, the method's own entry winning")
		PIt("reads response headers the spec declares, with their marks")
		PIt("matches header names without regard to upper or lower case")
	})

	Context("reading the secrets", func() {
		PIt("names Authorization as a secret for an http bearer or basic scheme")
		PIt("names the header of an apiKey scheme in a header as a secret, e.g. Authorization for Talon.One")
		PIt("names the query parameter of an apiKey scheme in the query as a secret")
		PIt("names the cookie of an apiKey scheme in a cookie as a secret")
		PIt("names secrets from security schemes used by one operation as well as from the global security section")
	})
})
