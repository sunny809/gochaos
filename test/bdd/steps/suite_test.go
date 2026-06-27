package steps

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/cucumber/godog"
)

func TestBDD(t *testing.T) {
	t.Parallel()

	// Determine feature paths relative to the test file
	// This makes the test work regardless of the working directory.
	pkgDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("cannot get working dir: %v", err)
	}
	featurePath := filepath.Join(pkgDir, "..", "features", "core")

	suite := godog.TestSuite{
		Name: "gmock",
		Options: &godog.Options{
			Paths:    []string{featurePath},
			Format:   "pretty",
			Strict:   false,
			NoColors: true,
		},
		ScenarioInitializer: func(ctx *godog.ScenarioContext) {
			tc := &TestContext{}
			ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
				if err := tc.BeforeScenario(); err != nil {
					return ctx, err
				}
				return ctx, nil
			})
			ctx.After(func(ctx context.Context, sc *godog.Scenario, err error) (context.Context, error) {
				tc.AfterScenario()
				return ctx, err
			})
			registerCoreSteps(ctx, tc)
			registerChaosSteps(ctx, tc)
			registerAdminSteps(ctx, tc)
		},
	}

	status := suite.Run()

	if status != 0 {
		t.Fatalf("BDD tests failed (exit code %d)", status)
	}
}
