package e2etestrun

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openmcp-project/controller-utils/pkg/clusters"
	testutils "github.com/openmcp-project/controller-utils/pkg/testing"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openmcp-project/platform-service-test-runner/api/v1alpha1"
	"github.com/openmcp-project/platform-service-test-runner/internal/runner"

	"github.com/openmcp-project/platform-service-test-runner/api/install"
)

const (
	platformCluster   = "platform"
	e2eTestReconciler = "e2etestrun-reconciler"
)

var platformScheme = install.InstallOperatorAPIsPlatform(runtime.NewScheme())

func e2eTestRunTestSetup(runSuccess, cleanupSuccess bool, testDirPathSegments ...string) *testutils.ComplexEnvironment {
	registry := runner.NewTestRegistry()
	registry.RegisterTestCase("fakeTest", &fakeTest{runSuccess: runSuccess, cleanupSuccess: cleanupSuccess})

	envBuilder :=
		testutils.NewComplexEnvironmentBuilder().
			WithFakeClient(platformCluster, platformScheme).
			WithInitObjectPath(platformCluster, testDirPathSegments...).
			WithReconcilerConstructor(e2eTestReconciler, func(c ...client.Client) reconcile.Reconciler {
				return NewE2ETestRunReconciler(
					clusters.NewTestClusterFromClient(platformCluster, c[0]),
					nil,
					"identity",
					registry,
				)
			}, platformCluster)

	return envBuilder.Build()
}

var _ = Describe("E2ETestSpecificationReconciler", func() {

	It("should do nothing if no E2ETestRun resource exists", func() {
		env := e2eTestRunTestSetup(true, true, "testdata", "test-01")

		Expect(env.Client(platformCluster).DeleteAllOf(env.Ctx, &v1alpha1.E2ETestRun{})).To(Succeed())

		testRun := &v1alpha1.E2ETestRun{}
		env.ShouldReconcile(e2eTestReconciler, testutils.RequestFromObject(testRun))

		// Verify that neither Run nor Cleanup was called
		tc, _ := env.Reconciler("e2etestrun-reconciler").(*E2ETestRunReconciler).testRegistry.GetTestCase("fakeTest")
		fakeTestInstance := tc.(*fakeTest)
		Expect(fakeTestInstance.runCalled).To(BeFalse())
		Expect(fakeTestInstance.cleanupCalled).To(BeFalse())
	})

	It("should err if test case not found", func() {
		env := e2eTestRunTestSetup(true, true, "testdata", "test-02")

		testRun := &v1alpha1.E2ETestRun{}
		Expect(env.Client(platformCluster).Get(env.Ctx, client.ObjectKey{Name: "test-run-02", Namespace: "test-run-02-ns"}, testRun)).To(Succeed())

		_, err := env.Reconcilers[e2eTestReconciler].Reconcile(env.Ctx, testutils.RequestFromObject(testRun))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("test not found in registry"))
	})

	It("should skip run and do cleanup if test already passed", func() {
		env := e2eTestRunTestSetup(true, true, "testdata", "test-03")

		testRun := &v1alpha1.E2ETestRun{}
		Expect(env.Client(platformCluster).Get(env.Ctx, client.ObjectKey{Name: "test-run-03", Namespace: "test-run-03-ns"}, testRun)).To(Succeed())

		_ = env.ShouldReconcile(e2eTestReconciler, testutils.RequestFromObject(testRun))

		// Verify that Run was not called but Cleanup was called
		tc, _ := env.Reconciler("e2etestrun-reconciler").(*E2ETestRunReconciler).testRegistry.GetTestCase("fakeTest")
		fakeTestInstance := tc.(*fakeTest)
		Expect(fakeTestInstance.runCalled).To(BeFalse())
		Expect(fakeTestInstance.cleanupCalled).To(BeTrue())
	})

	It("should exit run and cleanup if test already failed", func() {
		env := e2eTestRunTestSetup(true, true, "testdata", "test-04")

		testRun := &v1alpha1.E2ETestRun{}
		Expect(env.Client(platformCluster).Get(env.Ctx, client.ObjectKey{Name: "test-run-04", Namespace: "test-run-04-ns"}, testRun)).To(Succeed())

		_ = env.ShouldReconcile(e2eTestReconciler, testutils.RequestFromObject(testRun))

		// Verify that neither Run nor Cleanup were called
		tc, _ := env.Reconciler("e2etestrun-reconciler").(*E2ETestRunReconciler).testRegistry.GetTestCase("fakeTest")
		fakeTestInstance := tc.(*fakeTest)
		Expect(fakeTestInstance.runCalled).To(BeFalse())
		Expect(fakeTestInstance.cleanupCalled).To(BeFalse())
	})

	It("should  run and cleanup test case with config", func() {
		env := e2eTestRunTestSetup(true, true, "testdata", "test-05")

		testRun := &v1alpha1.E2ETestRun{}
		Expect(env.Client(platformCluster).Get(env.Ctx, client.ObjectKey{Name: "test-run-05", Namespace: "test-run-05-ns"}, testRun)).To(Succeed())

		_ = env.ShouldReconcile(e2eTestReconciler, testutils.RequestFromObject(testRun))

		// Verify the test passed
		Expect(env.Client(platformCluster).Get(env.Ctx, client.ObjectKey{Name: "test-run-05", Namespace: "test-run-05-ns"}, testRun)).To(Succeed())
		Expect(testRun.Status.TestCases).To(HaveLen(1))
		for _, testCase := range testRun.Status.TestCases {
			Expect(isTestCasePassed(testCase)).To(BeTrue())
		}

		// Verify config was passed correctly
		tc, _ := env.Reconciler("e2etestrun-reconciler").(*E2ETestRunReconciler).testRegistry.GetTestCase("fakeTest")
		fakeTestInstance := tc.(*fakeTest)
		Expect(fakeTestInstance.receivedConfig["identity"]).To(Equal("identity"))
		Expect(fakeTestInstance.receivedConfig["someKey"]).To(Equal("someValue"))
		// Verify that neither Run nor Cleanup were called
		Expect(fakeTestInstance.runCalled).To(BeTrue())
		Expect(fakeTestInstance.cleanupCalled).To(BeTrue())
	})

	It("should reconcile and run with error", func() {
		env := e2eTestRunTestSetup(false, true, "testdata", "test-06")

		testRun := &v1alpha1.E2ETestRun{}
		Expect(env.Client(platformCluster).Get(env.Ctx, client.ObjectKey{Name: "test-run-06", Namespace: "test-run-06-ns"}, testRun)).To(Succeed())

		_, err := env.Reconcilers[e2eTestReconciler].Reconcile(env.Ctx, testutils.RequestFromObject(testRun))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("some run error"))

		// Verify the test failed
		Expect(env.Client(platformCluster).Get(env.Ctx, client.ObjectKey{Name: "test-run-06", Namespace: "test-run-06-ns"}, testRun)).To(Succeed())
		Expect(testRun.Status.TestCases).To(HaveLen(1))
		for _, testCase := range testRun.Status.TestCases {
			Expect(isTestCaseFailed(testCase)).To(BeTrue())
		}

		// Verify that Run was called but not Cleanup
		tc, _ := env.Reconciler("e2etestrun-reconciler").(*E2ETestRunReconciler).testRegistry.GetTestCase("fakeTest")
		fakeTestInstance := tc.(*fakeTest)
		Expect(fakeTestInstance.runCalled).To(BeTrue())
		Expect(fakeTestInstance.cleanupCalled).To(BeFalse())
	})

	It("should reconcile and cleanup with error", func() {
		env := e2eTestRunTestSetup(true, false, "testdata", "test-06")

		testRun := &v1alpha1.E2ETestRun{}
		Expect(env.Client(platformCluster).Get(env.Ctx, client.ObjectKey{Name: "test-run-06", Namespace: "test-run-06-ns"}, testRun)).To(Succeed())

		_, err := env.Reconcilers[e2eTestReconciler].Reconcile(env.Ctx, testutils.RequestFromObject(testRun))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("some cleanup error"))

		// Verify the test run passed but cleanup failed
		Expect(env.Client(platformCluster).Get(env.Ctx, client.ObjectKey{Name: "test-run-06", Namespace: "test-run-06-ns"}, testRun)).To(Succeed())
		Expect(testRun.Status.TestCases).To(HaveLen(1))
		for _, testCase := range testRun.Status.TestCases {
			// Run should have passed
			Expect(isTestCasePassed(testCase)).To(BeTrue())
			// But cleanup should have failed
			Expect(isTestCaseCleanupFailed(testCase)).To(BeTrue())
		}

		// Verify that Run and Cleanup were called
		tc, _ := env.Reconciler("e2etestrun-reconciler").(*E2ETestRunReconciler).testRegistry.GetTestCase("fakeTest")
		fakeTestInstance := tc.(*fakeTest)
		Expect(fakeTestInstance.runCalled).To(BeTrue())
		Expect(fakeTestInstance.cleanupCalled).To(BeTrue())
	})

})
