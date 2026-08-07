package runner

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openmcp-project/platform-service-test-runner/internal/utils"

	"github.com/openmcp-project/platform-service-test-runner/api/v1alpha1"
)

const (
	labelTestCase  = "test-case"
	configIdentity = "identity"

	configContextTimeout = "contextTimeout"
	configPollInterval   = "pollInterval"
	configPollTimeout    = "pollTimeout"

	defaultContextTimeout = 60 * time.Minute
	defaultPollInterval   = 10 * time.Second
	defaultPollTimeout    = 10 * time.Minute
)

type Config map[string]interface{}
type Exports map[string]interface{}
type DebugInfo map[string]interface{}

// TestCase defines the interface for a modular E2E test case. Each test case must implement Run and Cleanup.
type TestCase interface {
	// StatusName returns the name used to identify this test case's status entry.
	// For test cases with a fixed name this is a constant; for test cases that can appear
	// multiple times (e.g. createService) it incorporates a discriminator from config (e.g. the manifest kind).
	StatusName(config Config) string

	// Run executes the test case. It receives the test run and config, and can return exports for other test cases, error details, and debug info.
	Run(ctx context.Context, run *v1alpha1.E2ETestRun, config Config) (Exports, DebugInfo, error)

	// Cleanup reverses the test case actions. Receives config and exports.
	Cleanup(ctx context.Context, run *v1alpha1.E2ETestRun, config Config) error
}

// GetStatus retrieves the TestCaseStatus for a given test case name from the list of test case statuses.
func GetStatus(name string, testCaseStati []v1alpha1.TestCaseStatus) (*v1alpha1.TestCaseStatus, bool) {
	for i, tC := range testCaseStati {
		if tC.Name == name {
			return &testCaseStati[i], true
		}
	}
	return nil, false
}

// ReadyCheckFunc is a function type that checks if an object is ready.
// It returns true if the object is ready, false otherwise.
type ReadyCheckFunc[T client.Object] func(obj T) bool

// WaitForReadyAndGet polls until the object is ready (as determined by the readyCheck function) and returns it.
// It uses generics to work with any client.Object type (Project, Workspace, MCP, etc.).
func WaitForReadyAndGet[T client.Object](
	ctx context.Context,
	k8sClient client.Client,
	name, namespace string,
	obj T,
	pollInterval time.Duration,
	pollTimeout time.Duration,
	readyCheck ReadyCheckFunc[T],
) (T, error) {
	err := wait.PollUntilContextTimeout(ctx, pollInterval, pollTimeout, true, func(ctx context.Context) (bool, error) {
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, obj); err != nil {
			return false, err
		}
		return readyCheck(obj), nil
	})
	return obj, err
}

// WaitForDeletion polls until the object is deleted (returns NotFound error).
// It uses generics to work with any client.Object type (Project, Workspace, MCP, etc.).
func WaitForDeletion[T client.Object](
	ctx context.Context,
	k8sClient client.Client,
	name, namespace string,
	obj T,
	pollInterval time.Duration,
	pollTimeout time.Duration,
) error {
	return wait.PollUntilContextTimeout(ctx, pollInterval, pollTimeout, true, func(ctx context.Context) (bool, error) {
		err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, obj)
		if errors.IsNotFound(err) {
			return true, nil // Resource is gone
		}
		if err != nil {
			return false, err // Unexpected error
		}
		return false, nil // Still exists, keep polling
	})
}

func GetContextTimeoutOrDefault(config Config) time.Duration {
	contextTimeout := defaultContextTimeout
	contextTimeoutConfig := utils.GetAsString(config, configContextTimeout)
	if contextTimeoutConfig != "" {
		if parsed, err := time.ParseDuration(contextTimeoutConfig); err == nil {
			contextTimeout = parsed
		}
	}
	return contextTimeout
}

func GetPollIntervalOrDefault(config Config) time.Duration {
	pollInterval := defaultPollInterval
	pollIntervalConfig := utils.GetAsString(config, configPollInterval)
	if pollIntervalConfig != "" {
		if parsed, err := time.ParseDuration(pollIntervalConfig); err == nil {
			pollInterval = parsed
		}
	}
	return pollInterval
}

func GetPollTimeoutOrDefault(config Config) time.Duration {
	pollTimeout := defaultPollTimeout
	pollTimeoutConfig := utils.GetAsString(config, configPollTimeout)
	if pollTimeoutConfig != "" {
		if parsed, err := time.ParseDuration(pollTimeoutConfig); err == nil {
			pollTimeout = parsed
		}
	}
	return pollTimeout
}
