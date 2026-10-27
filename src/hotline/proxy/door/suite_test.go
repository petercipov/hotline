package door_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestDoor(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Proxy Door Suite")
}
