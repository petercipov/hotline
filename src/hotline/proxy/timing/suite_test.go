package timing_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestTiming(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Proxy Timing Suite")
}
