package otel_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"testing"
)

func TestOtel(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "otel Suite")
}
