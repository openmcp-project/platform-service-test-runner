package e2etestspec

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openmcp-project/controller-utils/pkg/clusters"
	testutils "github.com/openmcp-project/controller-utils/pkg/testing"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openmcp-project/platform-service-test-runner/api/install"
	"github.com/openmcp-project/platform-service-test-runner/api/v1alpha1"
)

const (
	platformCluster = "platform"
)

var platformScheme = install.InstallOperatorAPIsPlatform(runtime.NewScheme())

func e2eTestSpecTestSetup(testDirPathSegments ...string) *testutils.Environment {
	env := testutils.NewEnvironmentBuilder().
		WithFakeClient(platformScheme).
		WithInitObjectPath(testDirPathSegments...).
		WithReconcilerConstructor(func(c client.Client) reconcile.Reconciler {
			return NewE2ETestSpecificationReconciler(clusters.NewTestClusterFromClient(platformCluster, c))
		}).
		Build()

	return env
}

var _ = Describe("E2ETestSpecificationReconciler", func() {

	It("should do nothing if no E2ETestSpec resource exists", func() {
		env := e2eTestSpecTestSetup("testdata", "test-01")

		// delete any existing DNSServiceConfig
		Expect(env.Client().DeleteAllOf(env.Ctx, &v1alpha1.E2ETestSpecification{})).To(Succeed())

		testSpec := &v1alpha1.E2ETestSpecification{}
		env.ShouldReconcile(testutils.RequestFromObject(testSpec))
	})

	It("should err out without schedule", func() {
		env := e2eTestSpecTestSetup("testdata", "test-01")

		testSpec := &v1alpha1.E2ETestSpecification{}
		Expect(env.Client().Get(env.Ctx, client.ObjectKey{Name: "test-specification-01", Namespace: "test-specification-01-ns"}, testSpec)).To(Succeed())

		_, err := env.Reconciler().Reconcile(env.Ctx, testutils.RequestFromObject(testSpec))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(errNoSchedule))
	})

	It("should reconcile every minute and create TestRun", func() {
		env := e2eTestSpecTestSetup("testdata", "test-02")

		testSpec := &v1alpha1.E2ETestSpecification{}
		Expect(env.Client().Get(env.Ctx, client.ObjectKey{Name: "test-specification-02", Namespace: "test-specification-02-ns"}, testSpec)).To(Succeed())

		result := env.ShouldReconcile(testutils.RequestFromObject(testSpec))
		Expect(result.RequeueAfter).To(BeNumerically(">", 0))
		Expect(result.RequeueAfter).To(BeNumerically("<=", time.Hour))

		testRunList := &v1alpha1.E2ETestRunList{}
		Expect(env.Client().List(env.Ctx, testRunList,
			client.InNamespace("test-specification-02-ns"),
			client.MatchingLabels{"test-runner.openmcp.cloud/specification": "test-specification-02"},
		)).To(Succeed())
		Expect(testRunList.Items).To(HaveLen(1))
		Expect(testRunList.Items[0].Spec.TestCases).To(HaveLen(3))
	})

	It("should reconcile every minute and create TestRun with config", func() {
		env := e2eTestSpecTestSetup("testdata", "test-03")

		testSpec := &v1alpha1.E2ETestSpecification{}
		Expect(env.Client().Get(env.Ctx, client.ObjectKey{Name: "test-specification-03", Namespace: "test-specification-03-ns"}, testSpec)).To(Succeed())

		result := env.ShouldReconcile(testutils.RequestFromObject(testSpec))
		Expect(result.RequeueAfter).To(BeNumerically(">", 0))
		Expect(result.RequeueAfter).To(BeNumerically("<=", time.Hour))

		testRunList := &v1alpha1.E2ETestRunList{}
		Expect(env.Client().List(env.Ctx, testRunList,
			client.InNamespace("test-specification-03-ns"),
			client.MatchingLabels{"test-runner.openmcp.cloud/specification": "test-specification-03"},
		)).To(Succeed())
		Expect(testRunList.Items).To(HaveLen(1))
		Expect(testRunList.Items[0].Spec.TestCases).To(HaveLen(3))
		Expect(testRunList.Items[0].Spec.TestCases[0].Config).To(HaveKey("chargingTarget"))
		Expect(testRunList.Items[0].Spec.TestCases[0].Config).To(HaveKey("chargingTargetType"))
	})

})
