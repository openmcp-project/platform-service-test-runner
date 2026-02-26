package e2etestrun

import (
	"context"
	"fmt"

	"github.com/openmcp-project/platform-service-test-runner/api/v1alpha1"
)

type fakeTest struct {
	runSuccess, cleanupSuccess bool
}

func (ft *fakeTest) Run(_ context.Context, _ *v1alpha1.E2ETestRun, _ map[string]string) (map[string]string, map[string]string, error) {
	if !ft.runSuccess {
		return make(map[string]string), nil, fmt.Errorf("some run error")
	}
	return make(map[string]string), nil, nil
}

func (ft *fakeTest) Cleanup(_ context.Context, _ *v1alpha1.E2ETestRun, _ map[string]string) error {
	if !ft.cleanupSuccess {
		return fmt.Errorf("some cleanup error")
	}
	return nil
}
