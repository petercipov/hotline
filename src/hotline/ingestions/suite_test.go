package ingestions_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestIngestions(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Ingestions Suite")
}
