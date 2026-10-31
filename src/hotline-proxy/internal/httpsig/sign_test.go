package httpsig_test

import (
	"encoding/base64"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"hotline-proxy/internal/httpsig"
)

func TestHTTPSig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "HTTP Signatures Suite")
}

// The signature base must follow RFC 9421 byte for byte, or no other tool
// can check a signature. The RFC's own examples are the test.
var _ = Describe("RFC 9421 signatures with HMAC-SHA256", func() {
	key, _ := base64.StdEncoding.DecodeString("uzvJfB4u3N0Jy4T7NZ75MDVcr8zSTInedJtkgcu46YW4XByzNJjxBdtjUkdJPBtbmHhIDi6pcl8jsasjlTMtDQ==")

	It("gives the signature of the RFC's example B.2.5", func() {
		signature, input := httpsig.HMACSHA256(key, []httpsig.Component{
			{Name: "date", Value: "Tue, 20 Apr 2021 02:07:55 GMT"},
			{Name: "@authority", Value: "example.com"},
			{Name: "content-type", Value: "application/json"},
		}, httpsig.Params{Created: 1618884473, KeyID: "test-shared-secret"})

		Expect(input).To(Equal(`("date" "@authority" "content-type");created=1618884473;keyid="test-shared-secret"`))
		Expect(signature).To(Equal("pxcQw6G3AjtMBQjwo8XzkZf/bws5LelbaMk5rGIGtE8="))
	})

	It("names a component's parameter and the algorithm in the input", func() {
		_, input := httpsig.HMACSHA256(key, []httpsig.Component{
			{Name: "@status", Value: "200"},
			{Name: "x-request-id", Param: "resp", Value: "r1"},
		}, httpsig.Params{Created: 1, KeyID: "k", Alg: "hmac-sha256"})

		Expect(input).To(Equal(`("@status" "x-request-id";resp);created=1;keyid="k";alg="hmac-sha256"`))
	})

	It("changes when any covered value changes", func() {
		params := httpsig.Params{Created: 1, KeyID: "k"}
		first, _ := httpsig.HMACSHA256(key, []httpsig.Component{{Name: "@method", Value: "GET"}}, params)
		second, _ := httpsig.HMACSHA256(key, []httpsig.Component{{Name: "@method", Value: "PUT"}}, params)
		Expect(first).NotTo(Equal(second))
	})
})
