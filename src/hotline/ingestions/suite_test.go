package ingestions_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"testing"
)

func TestIngestions(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Ingestions Suite")
}
