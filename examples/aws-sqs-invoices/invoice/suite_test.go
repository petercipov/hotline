package invoice_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestInvoice(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Invoice Suite")
}
