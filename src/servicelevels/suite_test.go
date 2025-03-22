package servicelevels_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"testing"
)

func TestServiceLevels(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Service Levels Suite")
}
