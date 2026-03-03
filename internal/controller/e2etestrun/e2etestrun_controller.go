package e2etestrun

import (
	"context"
	"fmt"

	"github.com/openmcp-project/controller-utils/pkg/clusters"
	"github.com/openmcp-project/controller-utils/pkg/logging"
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

	for _, testCaseSpec := range run.Spec.TestCases {
		testCase, found := r.testRegistry.GetTestCase(testCaseSpec.Name)
		if !found {
			log.Error(nil, "test not found in registry", "testName", testCaseSpec.Name)
			return ctrl.Result{}, fmt.Errorf("test not found in registry, testName %s", testCaseSpec.Name)
		}
		existingStatus, found := runner.GetStatus(testCaseSpec.Name, run.Status.TestCases)
		// if already passed -> skip
		if found && existingStatus.Status == TestStatusPassed {
			log.Info("Skipping already passed test", "testName", testCaseSpec.Name)
			continue
		}
		// if already failed -> skip the rest of the tests
		if found && existingStatus.Status == TestStatusFailed {
			log.Info("Skipping test and the rest of the tests due to previous failure", "testName", testCaseSpec.Name)
			return ctrl.Result{}, nil
		}

		log.Info("Running test", "testName", testCaseSpec.Name)
		testExports, debugInfo, err := testCase.Run(ctx, run, runner.Config{"identity": r.identity})
		if err != nil {
			log.Error(err, "error running test", "testName", testCaseSpec.Name)
			if statusErr := r.updateStatus(ctx, log, run, testCaseSpec, TestStatusFailed, testExports, debugInfo, err); statusErr != nil {
				return ctrl.Result{}, statusErr
			}
			return ctrl.Result{}, err
		}

		if statusErr := r.updateStatus(ctx, log, run, testCaseSpec, TestStatusPassed, testExports, debugInfo, err); statusErr != nil {
			return ctrl.Result{}, statusErr
		}
	}

	// if all tests passed, delete in backwards order to trigger cleanup
	for i := len(run.Spec.TestCases) - 1; i >= 0; i-- {
		testCase := run.Spec.TestCases[i]
		test, found := r.testRegistry.GetTestCase(testCase.Name)
		if !found {
			log.Error(nil, "test not found in registry during cleanup", "testName", testCase.Name)
			continue
		}
		log.Info("Running cleanup for test", "testName", testCase.Name)
		cleanupErr := test.Cleanup(ctx, run, nil)
		if cleanupErr != nil {
			existingStatus, _ := runner.GetStatus(testCase.Name, run.Status.TestCases)
			existingStatus.Status = TestStatusFailed
			existingStatus.Error = cleanupErr.Error()
			// Update status after cleanup failure
			if updateErr := r.platformCluster.Client().Status().Update(ctx, run); updateErr != nil {
				log.Error(updateErr, "unable to update E2ETestRun status")
				return ctrl.Result{}, updateErr
			}
			return ctrl.Result{}, cleanupErr
		}
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *E2ETestRunReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&testingopenmcpcloudv1alpha1.E2ETestRun{}).
		Named("e2etestrun").
		Complete(r)
}

func (r *E2ETestRunReconciler) updateStatus(ctx context.Context, log logging.Logger, run *testingopenmcpcloudv1alpha1.E2ETestRun, testCase testingopenmcpcloudv1alpha1.TestCase, status string, exports runner.Exports, debugIngo runner.DebugInfo, err error) error {
	tcStatus, statusErr := r.toApiTestCaseStatus(testCase.Name, status, exports, debugIngo, err)
	if statusErr != nil {
		log.Error(err, "error creating test case status", "testName", testCase.Name)
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
	testStatus := testingopenmcpcloudv1alpha1.TestCaseStatus{
		Name:      name,
		Status:    status,
		Exports:   expJSON,
		DebugInfo: diJSON,
	}
	if err != nil {
		testStatus.Error = err.Error()
	}
	return testStatus, nil
}
