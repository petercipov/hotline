package config_test

import (
	. "github.com/onsi/ginkgo/v2"
)

// Slice 1 test cases, written first for review. Each one becomes a real
// It block, test first, when slice 1 is built. "Q1" and so on point to the
// open questions in docs/proxy-slice-1.md.
var _ = Describe("Loading the config file", func() {
	Context("normal cases", func() {
		PIt("reads the proxy listen address")
		PIt("reads one or more OTLP exporters with their endpoints")
		PIt("uses cumulative temporality when an exporter does not say, and delta when it says delta")
		PIt("reads each integration with its apis, their hosts and the spec each api points to")
		PIt("finds spec files relative to the folder of the config file")
		PIt("turns TLS origination on when an api does not say")
		PIt("turns TLS origination off for an api that sets tlsOrigination to false")
		PIt("waits 60 seconds for the vendor when an api does not set vendorTimeout")
		PIt("uses the vendorTimeout an api sets")
		PIt("uses the built-in not-passed settings when the file has no not-passed entry: no TLS origination, any host")
		PIt("lets a not-passed entry override the built-in not-passed settings")
		PIt("loads the full example config from proxy-example together with both of its specs")
		PIt("gives each loaded config a version that every record carries as hotline.config.version (Q7)")
	})

	Context("edge cases", func() {
		PIt("accepts one api that lists several hosts")
		PIt("accepts the same host in two different integrations, each with its own spec")
		PIt("matches hosts without regard to upper or lower case")
		PIt("accepts a config with no integrations at all, so every call uses the not-passed settings")
	})

	Context("failure cases", func() {
		PIt("refuses a file that is not valid YAML and says where the problem is")
		PIt("refuses a config with no exporters")
		PIt("refuses two integrations with the same ID")
		PIt("refuses a host listed in two apis of the same integration")
		PIt("refuses an api whose spec file does not exist")
		PIt("refuses a spec that is not a valid OpenAPI 3 document")
		PIt("refuses a vendorTimeout that is zero, negative or not a duration")
		PIt("refuses an x-hotline-redact mark that is not true or false")
		PIt("refuses a not-passed entry that has an apis list (Q8)")
		PIt("refuses unknown fields, so a typo like tlsOrigintion is caught (Q8)")
		PIt("lists every problem it found, not only the first one")
		PIt("never puts a secret value into an error message")
	})
})
