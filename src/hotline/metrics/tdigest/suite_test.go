package tdigest_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestTDigest(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "TDigest Suite")
}
