package integrations_test

import (
	"hotline/integrations"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Entities", func() {
	It("should transform empty array", func() {
		id := integrations.ID("test")
		Expect(id.String()).To(Equal("test"))
	})

})
