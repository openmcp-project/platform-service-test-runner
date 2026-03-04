package e2etestrun

import (
	"context"
	"fmt"

	"github.com/openmcp-project/platform-service-test-runner/api/v1alpha1"
	"github.com/openmcp-project/platform-service-test-runner/internal/runner"
)

type fakeTest struct {
	runSuccess, cleanupSuccess bool
	receivedRun                *v1alpha1.E2ETestRun
	receivedConfig             runner.Config
}

func (ft *fakeTest) Run(_ context.Context, run *v1alpha1.E2ETestRun, config runner.Config) (runner.Exports, runner.DebugInfo, error) {
	ft.receivedRun = run
	ft.receivedConfig = config
	if !ft.runSuccess {
		return make(runner.Exports), nil, fmt.Errorf("some run error")
	}
	return make(runner.Exports), nil, nil
}

func (ft *fakeTest) Cleanup(_ context.Context, run *v1alpha1.E2ETestRun, config runner.Config) error {
	ft.receivedRun = run
	ft.receivedConfig = config
	if !ft.cleanupSuccess {
		return fmt.Errorf("some cleanup error")
	}
	return nil
}
