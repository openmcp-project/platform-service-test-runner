package e2etestrun

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/openmcp-project/controller-utils/pkg/clusters"
	"github.com/openmcp-project/controller-utils/pkg/conditions"
	"github.com/openmcp-project/controller-utils/pkg/logging"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openmcp-project/platform-service-test-runner/internal/utils"

	testingopenmcpcloudv1alpha1 "github.com/openmcp-project/platform-service-test-runner/api/v1alpha1"
	"github.com/openmcp-project/platform-service-test-runner/internal/runner"
)

const (
	ControllerName   = "E2ETestRunController"
	TestStatusPassed = "Passed"
	TestStatusFailed = "Failed"
)

// E2ETestRunReconciler reconciles a E2ETestRun object
type E2ETestRunReconciler struct {
	platformCluster *clusters.Cluster
	eventRecorder   events.EventRecorder
	identity        string
	testRegistry    *runner.TestRegistry
}

func NewE2ETestRunReconciler(platformCluster *clusters.Cluster, recorder events.EventRecorder, identity string, testRegistry *runner.TestRegistry) *E2ETestRunReconciler {
	return &E2ETestRunReconciler{
		platformCluster: platformCluster,
		eventRecorder:   recorder,
		identity:        identity,
		testRegistry:    testRegistry,
	}
}

// +kubebuilder:rbac:groups=testing.openmcp.cloud.test-runner.openmcp.cloud,resources=e2etestruns,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=testing.openmcp.cloud.test-runner.openmcp.cloud,resources=e2etestruns/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=testing.openmcp.cloud.test-runner.openmcp.cloud,resources=e2etestruns/finalizers,verbs=update

func (r *E2ETestRunReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logging.FromContextOrPanic(ctx).WithName(ControllerName)

	run := &testingopenmcpcloudv1alpha1.E2ETestRun{}
	if err := r.platformCluster.Client().Get(ctx, req.NamespacedName, run); err != nil {
		log.Error(err, "unable to fetch E2ETestRun")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Run all test cases
	if err := r.runTestCases(ctx, log, run); err != nil {
		return ctrl.Result{}, err
	}

	// Clean up test resources in reverse order
	if err := r.cleanupTestCases(ctx, log, run); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// runTestCases executes all test cases in the test run
func (r *E2ETestRunReconciler) runTestCases(ctx context.Context, log logging.Logger, run *testingopenmcpcloudv1alpha1.E2ETestRun) error {
	for _, testCaseSpec := range run.Spec.TestCases {
		testCase, found := r.testRegistry.GetTestCase(testCaseSpec.Name)
		if !found {
			log.Error(nil, "test not found in registry", "testName", testCaseSpec.Name)
			return fmt.Errorf("test not found in registry, testName %s", testCaseSpec.Name)
		}

		config, err := readConfig(testCaseSpec.Config)
		if err != nil {
			log.Error(err, "error reading test case config", "testName", testCaseSpec.Name)
			if statusErr := r.updateStatusAfterRun(ctx, log, run, testCaseSpec.Name, TestStatusFailed, nil, nil, err); statusErr != nil {
				return statusErr
			}
			return err
		}
		config["identity"] = r.identity

		statusName := testCase.StatusName(config)

		existingStatus, found := runner.GetStatus(statusName, run.Status.TestCases)
		// if already passed -> skip
		if found && isTestCasePassed(*existingStatus) {
			log.Info("Skipping already passed test", "testName", statusName)
			continue
		}
		// if already failed -> skip the rest of the tests
		if found && isTestCaseFailed(*existingStatus) {
			log.Info("Skipping test and the rest of the tests due to previous failure", "testName", statusName)
			return nil
		}

		log.Info("Running test", "testName", statusName)
		testExports, debugInfo, err := testCase.Run(ctx, run, config)
		if err != nil {
			log.Error(err, "error running test", "testName", statusName)
			if statusErr := r.updateStatusAfterRun(ctx, log, run, statusName, TestStatusFailed, testExports, debugInfo, err); statusErr != nil {
				return statusErr
			}
			return err
		}

		if statusErr := r.updateStatusAfterRun(ctx, log, run, statusName, TestStatusPassed, testExports, debugInfo, err); statusErr != nil {
			return statusErr
		}
	}
	return nil
}

// cleanupTestCases cleans up test resources in reverse order after all tests have run
func (r *E2ETestRunReconciler) cleanupTestCases(ctx context.Context, log logging.Logger, run *testingopenmcpcloudv1alpha1.E2ETestRun) error {
	for i := len(run.Spec.TestCases) - 1; i >= 0; i-- {
		testCaseSpec := run.Spec.TestCases[i]
		test, found := r.testRegistry.GetTestCase(testCaseSpec.Name)
		if !found {
			log.Error(nil, "test not found in registry during cleanup", "testName", testCaseSpec.Name)
			continue
		}

		config, err := readConfig(testCaseSpec.Config)
		if err != nil {
			log.Error(err, "error reading test case config during cleanup", "testName", testCaseSpec.Name)
			return err
		}
		config["identity"] = r.identity

		statusName := test.StatusName(config)

		existingStatus, found := runner.GetStatus(statusName, run.Status.TestCases)
		// if test case did not pass -> abort cleanup
		if !found || !isTestCasePassed(*existingStatus) {
			log.Info("Skipping cleanup", "testName", statusName)
			break
		}
		// if already cleaned up successfully -> skip
		if isTestCaseCleanupSucceeded(*existingStatus) {
			log.Info("Cleanup already completed, skipping", "testName", statusName)
			continue
		}

		log.Info("Running cleanup for test", "testName", statusName)
		cleanupErr := test.Cleanup(ctx, run, config)

		// Update test case status condition based on cleanup result
		if err := r.updateStatusAfterCleanup(ctx, log, run, statusName, cleanupErr); err != nil {
			return err
		}

		if cleanupErr != nil {
			return cleanupErr
		}
	}
	return nil
}

// updateStatusAfterCleanup updates the test case status condition after cleanup, indicating whether cleanup succeeded or if there was an error.
// It ensures that the status is updated regardless of cleanup success or failure, and logs any errors encountered during status update.
func (r *E2ETestRunReconciler) updateStatusAfterCleanup(ctx context.Context, log logging.Logger, run *testingopenmcpcloudv1alpha1.E2ETestRun, testCaseName string, cleanupErr error) error {
	// Find the test case status and update condition
	status, found := runner.GetStatus(testCaseName, run.Status.TestCases)
	if !found {
		statusErr := errors.New("unable to find test case status for cleanup update")
		log.Error(statusErr, "test case status not found during cleanup update", "testName", testCaseName)
		return statusErr
	}
	if cleanupErr != nil {
		// Update cleanup condition with error
		setTestCaseCondition(
			status,
			testingopenmcpcloudv1alpha1.TestCaseConditionCleanupCompleted,
			metav1.ConditionFalse,
			testingopenmcpcloudv1alpha1.TestCaseReasonCleanupError,
			cleanupErr.Error(),
		)
	} else {
		// Update cleanup condition to reflect successful cleanup
		setTestCaseCondition(
			status,
			testingopenmcpcloudv1alpha1.TestCaseConditionCleanupCompleted,
			metav1.ConditionTrue,
			testingopenmcpcloudv1alpha1.TestCaseReasonCleanupSuccess,
			"Test case cleanup completed successfully",
		)
	}

	// Update status after cleanup (whether success or failure)
	if updateErr := r.platformCluster.Client().Status().Update(ctx, run); updateErr != nil {
		log.Error(updateErr, "unable to update E2ETestRun status")
		return updateErr
	}

	return nil
}

// updateStatusAfterRun updates the test case status after running a test case, including exports, debug info, and error if any.
// It also updates the overall test run status and stops further execution if a test fails.
func (r *E2ETestRunReconciler) updateStatusAfterRun(
	ctx context.Context,
	log logging.Logger,
	run *testingopenmcpcloudv1alpha1.E2ETestRun,
	statusName string,
	status string,
	exports runner.Exports,
	debugIngo runner.DebugInfo,
	err error) error {
	tcStatus, statusErr := r.toApiTestCaseStatus(statusName, status, exports, debugIngo, err)
	if statusErr != nil {
		log.Error(err, "error creating test case status", "testName", statusName)
		return statusErr
	}
	run.Status.TestCases = append(run.Status.TestCases, tcStatus)
	// Update status and stop running further tests if any test fails.
	if updateErr := r.platformCluster.Client().Status().Update(ctx, run); updateErr != nil {
		log.Error(updateErr, "unable to update E2ETestRun status")
		return updateErr
	}
	return err
}

// toApiTestCaseStatus is a helper function to create a TestCaseStatus with proper JSON marshaling
func (r *E2ETestRunReconciler) toApiTestCaseStatus(name, status string, exports, debugInfo map[string]interface{}, err error) (testingopenmcpcloudv1alpha1.TestCaseStatus, error) {
	expJSON, marshallErr := utils.MarshalToRawMessage(exports)
	if marshallErr != nil {
		return testingopenmcpcloudv1alpha1.TestCaseStatus{}, err
	}
	diJSON, marshallErr := utils.MarshalToRawMessage(debugInfo)
	if marshallErr != nil {
		return testingopenmcpcloudv1alpha1.TestCaseStatus{}, err
	}

	var condStatus metav1.ConditionStatus
	var reason, message string

	if status == TestStatusPassed {
		condStatus = metav1.ConditionTrue
		reason = testingopenmcpcloudv1alpha1.TestCaseReasonPassed
		message = "Test case run completed successfully"
	} else {
		condStatus = metav1.ConditionFalse
		reason = testingopenmcpcloudv1alpha1.TestCaseReasonFailed
		if err != nil {
			message = err.Error()
		} else {
			message = "Test case run failed"
		}
	}

	conds, _ := conditions.ConditionUpdater(nil, false).
		UpdateCondition(testingopenmcpcloudv1alpha1.TestCaseConditionRunCompleted, condStatus, 0, reason, message).
		Conditions()

	testStatus := testingopenmcpcloudv1alpha1.TestCaseStatus{
		Name:       name,
		Exports:    expJSON,
		DebugInfo:  diJSON,
		Conditions: conds,
	}
	return testStatus, nil
}

// isTestCasePassed checks if a test case run has passed by examining its RunCompleted condition
func isTestCasePassed(status testingopenmcpcloudv1alpha1.TestCaseStatus) bool {
	cond := conditions.GetCondition(status.Conditions, testingopenmcpcloudv1alpha1.TestCaseConditionRunCompleted)
	return cond != nil && cond.Status == metav1.ConditionTrue
}

// isTestCaseFailed checks if a test case run has failed by examining its RunCompleted condition
func isTestCaseFailed(status testingopenmcpcloudv1alpha1.TestCaseStatus) bool {
	cond := conditions.GetCondition(status.Conditions, testingopenmcpcloudv1alpha1.TestCaseConditionRunCompleted)
	return cond != nil && cond.Status == metav1.ConditionFalse
}

// isTestCaseCleanupFailed checks if a test case cleanup has failed by examining its CleanupCompleted condition
func isTestCaseCleanupFailed(status testingopenmcpcloudv1alpha1.TestCaseStatus) bool {
	cond := conditions.GetCondition(status.Conditions, testingopenmcpcloudv1alpha1.TestCaseConditionCleanupCompleted)
	return cond != nil && cond.Status == metav1.ConditionFalse
}

// isTestCaseCleanupSucceeded checks if a test case cleanup has already completed successfully.
func isTestCaseCleanupSucceeded(status testingopenmcpcloudv1alpha1.TestCaseStatus) bool {
	cond := conditions.GetCondition(status.Conditions, testingopenmcpcloudv1alpha1.TestCaseConditionCleanupCompleted)
	return cond != nil && cond.Status == metav1.ConditionTrue
}

// setTestCaseCondition updates or adds a condition to a test case status using the conditions updater
func setTestCaseCondition(status *testingopenmcpcloudv1alpha1.TestCaseStatus, conditionType string, conditionStatus metav1.ConditionStatus, reason, message string) {
	updatedConditions, _ := conditions.ConditionUpdater(status.Conditions, false).
		UpdateCondition(conditionType, conditionStatus, 0, reason, message).
		Conditions()
	status.Conditions = updatedConditions
}

// readConfig is a helper function to unmarshal the test case config from JSON RawMessage to runner.Config struct
func readConfig(config json.RawMessage) (runner.Config, error) {
	cfg := runner.Config{}
	if config == nil {
		return cfg, nil
	}
	if err := json.Unmarshal(config, &cfg); err != nil {
		return nil, fmt.Errorf("error unmarshalling test case config: %w", err)
	}
	return cfg, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *E2ETestRunReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&testingopenmcpcloudv1alpha1.E2ETestRun{}).
		Named("e2etestrun").
		Complete(r)
}
