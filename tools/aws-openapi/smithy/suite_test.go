package smithy_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSmithy(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Smithy to OpenAPI Suite")
}
