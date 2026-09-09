package tdlatencyhistogram_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestTDLatencyHistogram(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "TD Latency Histogram Suite")
}
