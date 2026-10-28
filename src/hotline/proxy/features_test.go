package proxy_test

import (
	"testing"

	"github.com/cucumber/godog"
)

// TestFeatures runs the Gherkin files in ./features against the proxy as the
// system under test. Steps without a definition yet are reported as
// undefined and do not fail the run; once slice 1 is built, each step gets a
// definition and the run turns strict.
func TestFeatures(t *testing.T) {
	suite := godog.TestSuite{
		Name:                "hotline-proxy",
		ScenarioInitializer: func(*godog.ScenarioContext) {},
		Options: &godog.Options{
			Format:   "progress",
			Paths:    []string{"features"},
			Tags:     "~@e2e",
			TestingT: t,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("feature files failed")
	}
}
