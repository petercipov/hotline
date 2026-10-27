package record_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestRecord(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Proxy Record Suite")
}
