package digestcompare_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestDigestCompare(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Digest Compare Suite")
}
