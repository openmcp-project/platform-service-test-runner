package runner

import (
	"context"

	"github.com/openmcp-project/platform-service-test-runner/api/v1alpha1"
)

const (
	labelTestCase  = "test-case"
	configIdentity = "identity"
)

// TestCase defines the interface for a modular E2E test case. Each test case must implement Run and Cleanup.
type TestCase interface {
	// Run executes the test case. It receives the test run and config, and can return exports for other test cases, error details, and debug info.
	Run(ctx context.Context, run *v1alpha1.E2ETestRun, config map[string]string) (map[string]string, map[string]string, error)

	// Cleanup reverses the test case actions. Receives config and exports.
	Cleanup(ctx context.Context, run *v1alpha1.E2ETestRun, config map[string]string) error
}
