package e2etestspec

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/openmcp-project/controller-utils/pkg/clusters"
	"github.com/openmcp-project/controller-utils/pkg/logging"
	"github.com/robfig/cron/v3"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	testingopenmcpcloudv1alpha1 "github.com/openmcp-project/platform-service-test-runner/api/v1alpha1"
)

const (
	controllerName  = "E2ETestSpecificationController"
	labelTestRunner = "test-runner.openmcp.cloud/specification"

	errNoSchedule      = "no schedule defined in the specification, skipping execution"
	errInvalidSchedule = "invalid schedule format"
	errNoTestCases     = "no test cases defined in the specification"
)

// E2ETestSpecificationReconciler reconciles a E2ETestSpecification object
type E2ETestSpecificationReconciler struct {
	PlatformCluster *clusters.Cluster
}

func NewE2ETestSpecificationReconciler(platformCluster *clusters.Cluster) *E2ETestSpecificationReconciler {
	return &E2ETestSpecificationReconciler{
		PlatformCluster: platformCluster,
	}
}

var _ reconcile.Reconciler = &E2ETestSpecificationReconciler{}

// +kubebuilder:rbac:groups=testing.openmcp.cloud.test-runner.openmcp.cloud,resources=e2etestspecifications,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=testing.openmcp.cloud.test-runner.openmcp.cloud,resources=e2etestspecifications/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=testing.openmcp.cloud.test-runner.openmcp.cloud,resources=e2etestspecifications/finalizers,verbs=update

func (r *E2ETestSpecificationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logging.FromContextOrPanic(ctx).WithName(controllerName)

	// fetch specification
	var testSpec testingopenmcpcloudv1alpha1.E2ETestSpecification
	if err := r.PlatformCluster.Client().Get(ctx, req.NamespacedName, &testSpec); err != nil {
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("unable to get resource '%s' from cluster: %w", req.String(), err)
	}

	if len(testSpec.Spec.TestCases) == 0 {
		log.Info("No test cases defined in the specification, skipping execution")
		return ctrl.Result{}, nil
	}

	if testSpec.Spec.Schedule == "" {
		log.Info(errNoSchedule)
		return ctrl.Result{}, errors.New(errNoSchedule)
	}

	// Check if it's time to run based on the cron schedule
	schedule, err := cron.ParseStandard(testSpec.Spec.Schedule)
	if err != nil {
		log.Error(err, errInvalidSchedule, "schedule", testSpec.Spec.Schedule)
		return ctrl.Result{}, err
	}

	now := time.Now()
	nextRun := schedule.Next(now.Add(-1 * time.Minute))

	// Check if we're within the execution window (1 minute tolerance)
	if now.Before(nextRun) || now.Sub(nextRun) > time.Minute {
		requeueAfter := time.Until(nextRun)
		log.Debug("Not scheduled to run yet", "nextRun", nextRun, "requeueAfter", requeueAfter)
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	log.Info("Executing test specification", "name", testSpec.Name, "namespace", testSpec.Namespace)

	// Create a new E2ETestRun resource for each run
	run := testingopenmcpcloudv1alpha1.E2ETestRun{
		ObjectMeta: v1.ObjectMeta{
			Name:      fmt.Sprintf("run-%d", time.Now().UnixMilli()),
			Namespace: testSpec.Namespace,
			Labels: map[string]string{
				labelTestRunner: testSpec.Name,
			},
		},
		Spec: testingopenmcpcloudv1alpha1.E2ETestRunSpec{
			Runner: testingopenmcpcloudv1alpha1.Runner{
				Version: "v0.0.1", // todo use a real version
				Args:    []string{},
			},
			TestCases: testSpec.Spec.TestCases,
		},
	}

	if err := r.PlatformCluster.Client().Create(ctx, &run); err != nil {
		log.Error(err, "failed to create E2ETestRun")
		return ctrl.Result{}, err
	}

	// Requeue for next scheduled run
	nextRun = schedule.Next(time.Now())
	return ctrl.Result{RequeueAfter: time.Until(nextRun)}, nil

}

// SetupWithManager sets up the controller with the Manager.
func (r *E2ETestSpecificationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&testingopenmcpcloudv1alpha1.E2ETestSpecification{}).
		Named("e2etestspecification").
		Complete(r)
}
