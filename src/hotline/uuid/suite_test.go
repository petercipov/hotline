package uuid_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"testing"
)

func TestUUIDs(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "UUIS Suite")
}
