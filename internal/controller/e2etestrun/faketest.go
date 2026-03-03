package e2etestrun

import (
	"context"
	"fmt"

	"github.com/openmcp-project/platform-service-test-runner/api/v1alpha1"
	"github.com/openmcp-project/platform-service-test-runner/internal/runner"
)

type fakeTest struct {
	runSuccess, cleanupSuccess bool
}

func (ft *fakeTest) Run(_ context.Context, _ *v1alpha1.E2ETestRun, _ runner.Config) (runner.Exports, runner.DebugInfo, error) {
	if !ft.runSuccess {
		return make(runner.Exports), nil, fmt.Errorf("some run error")
	}
	return make(runner.Exports), nil, nil
}

func (ft *fakeTest) Cleanup(_ context.Context, _ *v1alpha1.E2ETestRun, _ runner.Config) error {
	if !ft.cleanupSuccess {
		return fmt.Errorf("some cleanup error")
	}
	return nil
}
