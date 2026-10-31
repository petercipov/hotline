package spec_test

import (
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"hotline-proxy/internal/spec"
)

// The request is checked while the proxy waits for the vendor, and the
// reply once it has arrived, so each side can be checked on its own
// (Peter, 2026-10-31).
var _ = Describe("Checking each side on its own", func() {
	var s *spec.Spec
	e := spec.Exchange{
		Method: "GET", EscapedPath: "/a", RawQuery: "n=x",
		Status: 418, ReplyHeader: http.Header{},
	}
	BeforeEach(func() {
		s = parsed(route("parameters:\n  - { name: n, in: query, schema: { type: integer } }\nresponses:\n  \"200\": { description: ok }\n"))
	})

	It("checks only the request", func() {
		got := s.CheckRequest(e)
		Expect(written(got.Request)).To(Equal("query n type integer"))
		Expect(got.Reply).To(BeEmpty())
	})

	It("checks only the reply", func() {
		got := s.CheckReply(e)
		Expect(got.Request).To(BeEmpty())
		Expect(written(got.Reply)).To(Equal("status 418 not_in_spec"))
	})

	It("checks both sides together", func() {
		got := s.Check(e)
		Expect(written(got.Request)).To(Equal("query n type integer"))
		Expect(written(got.Reply)).To(Equal("status 418 not_in_spec"))
	})

	It("says a call with no route was skipped, on each side", func() {
		other := spec.Exchange{Method: "GET", EscapedPath: "/b", Status: 200}
		Expect(s.CheckRequest(other).Skipped).To(Equal("no_spec_route"))
		Expect(s.CheckReply(other).Skipped).To(Equal("no_spec_route"))
	})

	It("keeps the request's reason when both sides skipped part", func() {
		s = parsed(route("requestBody:\n  content:\n    application/json: {}\nresponses:\n  \"200\":\n    description: ok\n    content:\n      application/json: {}\n"))
		got := s.Check(spec.Exchange{
			Method: "GET", EscapedPath: "/a",
			Header: http.Header{"Content-Type": {"application/json"}}, Body: spec.Body{TooLarge: true},
			Status: 200, ReplyHeader: http.Header{"Content-Type": {"text/event-stream"}},
		})
		Expect(got.Skipped).To(Equal("body_too_large"))
	})
})
