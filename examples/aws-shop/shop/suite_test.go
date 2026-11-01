package shop_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestShop(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Shop Suite")
}
