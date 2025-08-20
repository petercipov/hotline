package servicelevels_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestServiceLevels(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Service Levels Suite")
}
