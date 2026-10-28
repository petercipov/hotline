package config_test

import (
	. "github.com/onsi/ginkgo/v2"
)

// Slice 1 test cases, written first for review, in given, when, then form:
// Context holds the given, When the action and It the expected result.
// Each one becomes a real It block, test first, when slice 1 is built.
// "Q1" and so on point to the open questions in docs/proxy-slice-1.md.
var _ = Describe("Loading the config file", func() {
	Context("given a valid config file", func() {
		When("it is loaded", func() {
			PIt("then the proxy listen address is read")
			PIt("then every OTLP exporter is read with its endpoint")
			PIt("then every integration is read with its apis, their hosts and the spec each api points to")
			PIt("then spec files are found relative to the folder of the config file")
			PIt("then the config gets a version that every record carries as hotline.config.version (Q7)")
		})
	})

	Context("given an exporter", func() {
		When("it does not set temporality", func() {
			PIt("then its temporality is cumulative")
		})
		When("it sets temporality to delta", func() {
			PIt("then its temporality is delta")
		})
	})

	Context("given an api", func() {
		When("it does not set tlsOrigination", func() {
			PIt("then TLS origination is on")
		})
		When("it sets tlsOrigination to false", func() {
			PIt("then TLS origination is off")
		})
		When("it does not set vendorTimeout", func() {
			PIt("then the proxy waits 60 seconds for the vendor")
		})
		When("it sets vendorTimeout", func() {
			PIt("then that timeout is used")
		})
		When("it lists several hosts", func() {
			PIt("then every host uses that api")
		})
		When("its hosts are written in upper case", func() {
			PIt("then they match calls without regard to upper or lower case")
		})
	})

	Context("given the reserved not-passed ID", func() {
		When("the file has no not-passed entry", func() {
			PIt("then the built-in settings apply: no TLS origination, any host")
		})
		When("the file has a not-passed entry", func() {
			PIt("then its settings override the built-in ones")
		})
		When("the not-passed entry has an apis list", func() {
			PIt("then the config is refused (Q8)")
		})
	})

	Context("given the full example config from proxy-example", func() {
		When("it is loaded", func() {
			PIt("then it loads together with both of its specs")
		})
	})

	Context("given two integrations", func() {
		When("both list the same host, each with its own spec", func() {
			PIt("then both are accepted")
		})
		When("both have the same ID", func() {
			PIt("then the config is refused")
		})
	})

	Context("given a config with no integrations", func() {
		When("it is loaded", func() {
			PIt("then it is accepted and every call uses the not-passed settings")
		})
	})

	Context("given a broken config", func() {
		When("the file is not valid YAML", func() {
			PIt("then it is refused with the place of the problem")
		})
		When("there are no exporters", func() {
			PIt("then it is refused")
		})
		When("one integration lists the same host in two apis", func() {
			PIt("then it is refused")
		})
		When("an api points to a spec file that does not exist", func() {
			PIt("then it is refused")
		})
		When("a spec is not a valid OpenAPI 3 document", func() {
			PIt("then it is refused")
		})
		When("a vendorTimeout is zero, negative or not a duration", func() {
			PIt("then it is refused")
		})
		When("an x-hotline-redact mark is not true or false", func() {
			PIt("then it is refused")
		})
		When("a field name has a typo, such as tlsOrigintion", func() {
			PIt("then it is refused (Q8)")
		})
		When("it has several problems", func() {
			PIt("then every problem is listed, not only the first one")
		})
		When("any problem is reported", func() {
			PIt("then the message holds no secret value")
		})
	})
})
