package ddlatencyhistogram_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestLatencyHistogram(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Latency Histogram Suite")
}
