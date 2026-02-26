package e2etestrun

import (
	"context"
	"fmt"

	"github.com/openmcp-project/controller-utils/pkg/clusters"
	"github.com/openmcp-project/controller-utils/pkg/logging"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openmcp-project/platform-service-test-runner/internal/util"

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
		existingStatus, found := util.GetStatus(testCaseSpec.Name, run.Status.TestCases)

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
		testExports, _, err := testCase.Run(ctx, run, map[string]string{"identity": r.identity})
		if err != nil {
			log.Error(err, "error running test", "testName", testCaseSpec.Name)
			run.Status.TestCases = append(run.Status.TestCases, testingopenmcpcloudv1alpha1.TestCaseStatus{
				Name:      testCaseSpec.Name,
				Status:    TestStatusFailed,
				Exports:   testExports,
				Error:     err.Error(),
				DebugInfo: "",
			})
			// Update status and stop running further tests if any test fails.
			if err := r.platformCluster.Client().Status().Update(ctx, run); err != nil {
				log.Error(err, "unable to update E2ETestRun status")
				return ctrl.Result{}, err
			}
			// Record an event for the failure
			return ctrl.Result{}, err
		}

		run.Status.TestCases = append(run.Status.TestCases, testingopenmcpcloudv1alpha1.TestCaseStatus{
			Name:    testCaseSpec.Name,
			Status:  TestStatusPassed,
			Exports: testExports,
		})
		// Update status after each test case to have real-time visibility of the progress.
		if err := r.platformCluster.Client().Status().Update(ctx, run); err != nil {
			log.Error(err, "unable to update E2ETestRun status")
			return ctrl.Result{}, err
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
		err := test.Cleanup(ctx, run, nil)
		if err != nil {
			existingStatus, _ := util.GetStatus(testCase.Name, run.Status.TestCases)
			existingStatus.Status = TestStatusFailed
			existingStatus.Error = err.Error()

			// Update status after each test case to have real-time visibility of the progress.
			if err := r.platformCluster.Client().Status().Update(ctx, run); err != nil {
				log.Error(err, "unable to update E2ETestRun status")
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, err
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
