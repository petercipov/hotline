package clock_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/ginkgo/v2/types"
	. "github.com/onsi/gomega"
)

func TestClock(t *testing.T) {
	RegisterFailHandler(Fail)
	conf := types.NewDefaultSuiteConfig()
	conf.MustPassRepeatedly = 100
	RunSpecs(t, "Clock Suite", conf)
}
