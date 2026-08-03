package runner

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openmcp-project/openmcp-operator/api/common"
	omcpv2alpha1 "github.com/openmcp-project/openmcp-operator/api/core/v2alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/openmcp-project/platform-service-test-runner/internal/utils"

	"github.com/openmcp-project/controller-utils/pkg/logging"

	"github.com/openmcp-project/platform-service-test-runner/api/v1alpha1"
)

var _ = Describe("CreateControlPlaneTest", func() {
	var (
		testCtx                context.Context
		scheme                 *runtime.Scheme
		createControlPlaneTest *CreateControlPlaneTest
		testRun                *v1alpha1.E2ETestRun
		config                 Config
	)

	BeforeEach(func() {
		testCtx = logging.NewContext(context.Background(), logging.Discard())
		scheme = runtime.NewScheme()
		Expect(omcpv2alpha1.AddToScheme(scheme)).To(Succeed())
		Expect(v1alpha1.AddToScheme(scheme)).To(Succeed())

		testRun = &v1alpha1.E2ETestRun{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-run",
			},
		}
		config = Config{
			"pollTimeout": "1m",
		}
	})

	Describe("Run", func() {
		It("should create a ControlPlane, poll state and return exports", func() {
			created := false
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithInterceptorFuncs(interceptor.Funcs{
					Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
						created = true
						return c.Create(ctx, obj, opts...)
					},
					Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
						if err := c.Get(ctx, key, obj, opts...); err != nil {
							return err
						}
						// Simulate controller setting status.phase to Ready after creation
						if cp, ok := obj.(*omcpv2alpha1.ControlPlane); ok && created {
							cp.Status.Phase = common.StatusPhaseReady
						}
						return nil
					},
				}).
				Build()

			createControlPlaneTest = &CreateControlPlaneTest{OnboardingClient: fakeClient}
			exportJSON, _ := utils.MarshalToRawMessage(Exports{
				keyWorkspaceStatusNamespace: "workspace-namespace",
			})
			testRun.Status.TestCases = append(testRun.Status.TestCases, v1alpha1.TestCaseStatus{
				Name:    createWorkspace,
				Exports: exportJSON,
			})
			exports, _, err := createControlPlaneTest.Run(testCtx, testRun, config)

			Expect(err).NotTo(HaveOccurred())
			Expect(exports[keyControlPlaneName]).To(Equal("test-run-cp"))
			Expect(exports[keyControlPlaneNamespace]).To(Equal("workspace-namespace"))

			// Verify ControlPlane was created
			cp := &omcpv2alpha1.ControlPlane{}
			Expect(fakeClient.Get(testCtx, client.ObjectKey{Name: "test-run-cp", Namespace: "workspace-namespace"}, cp)).To(Succeed())
			Expect(cp.Labels["test-case"]).To(Equal(createControlPlane))
		})

		It("should poll state if ControlPlane already exists and return exports", func() {
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithInterceptorFuncs(interceptor.Funcs{
					Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
						return errors.NewAlreadyExists(schema.GroupResource{Group: "core.open-control-plane.io", Resource: "controlplanes"}, obj.GetName())
					},
					Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
						// Simulate existing ControlPlane is ready
						if cp, ok := obj.(*omcpv2alpha1.ControlPlane); ok {
							cp.Status.Phase = common.StatusPhaseReady
							return nil
						}
						return c.Get(ctx, key, obj, opts...)
					},
				}).
				Build()

			createControlPlaneTest = &CreateControlPlaneTest{OnboardingClient: fakeClient}
			exportJSON, _ := utils.MarshalToRawMessage(Exports{
				keyWorkspaceStatusNamespace: "workspace-namespace",
			})
			testRun.Status.TestCases = append(testRun.Status.TestCases, v1alpha1.TestCaseStatus{
				Name:    createWorkspace,
				Exports: exportJSON,
			})
			exports, _, err := createControlPlaneTest.Run(testCtx, testRun, config)

			Expect(err).NotTo(HaveOccurred())
			Expect(exports[keyControlPlaneName]).To(Equal("test-run-cp"))
			Expect(exports[keyControlPlaneNamespace]).To(Equal("workspace-namespace"))
		})

		It("should return error if ControlPlane creation fails", func() {
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithInterceptorFuncs(interceptor.Funcs{
					Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
						return errors.NewBadRequest("creation failed")
					},
				}).
				Build()

			createControlPlaneTest = &CreateControlPlaneTest{OnboardingClient: fakeClient}
			exportJSON, _ := utils.MarshalToRawMessage(Exports{
				keyWorkspaceStatusNamespace: "workspace-namespace",
			})
			testRun.Status.TestCases = append(testRun.Status.TestCases, v1alpha1.TestCaseStatus{
				Name:    createWorkspace,
				Exports: exportJSON,
			})
			_, _, err := createControlPlaneTest.Run(testCtx, testRun, config)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("creation failed"))
		})

	})

	Describe("Cleanup", func() {
		It("should delete ControlPlane successfully", func() {
			cp := &omcpv2alpha1.ControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-run-cp",
					Namespace: "workspace-namespace",
				},
			}

			deleteCount := 0
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(cp).
				WithInterceptorFuncs(interceptor.Funcs{
					Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
						if deleteCount > 0 {
							return errors.NewNotFound(schema.GroupResource{Group: "core.open-control-plane.io", Resource: "controlplanes"}, key.Name)
						}
						return c.Get(ctx, key, obj, opts...)
					},
					Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
						deleteCount++
						return c.Delete(ctx, obj, opts...)
					},
				}).
				Build()

			createControlPlaneTest = &CreateControlPlaneTest{OnboardingClient: fakeClient}
			exportJSON, _ := utils.MarshalToRawMessage(Exports{
				keyControlPlaneName:      "test-run-cp",
				keyControlPlaneNamespace: "workspace-namespace",
			})
			testRun.Status.TestCases = append(testRun.Status.TestCases, v1alpha1.TestCaseStatus{
				Name:    createControlPlane,
				Exports: exportJSON,
			})

			err := createControlPlaneTest.Cleanup(testCtx, testRun, nil)

			Expect(err).NotTo(HaveOccurred())
		})

		It("should succeed if ControlPlane already deleted", func() {
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithInterceptorFuncs(interceptor.Funcs{
					Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
						return errors.NewNotFound(schema.GroupResource{Group: "core.open-control-plane.io", Resource: "controlplanes"}, obj.GetName())
					},
				}).
				Build()

			createControlPlaneTest = &CreateControlPlaneTest{OnboardingClient: fakeClient}
			exportJSON, _ := utils.MarshalToRawMessage(Exports{
				keyControlPlaneName:      "test-run-cp",
				keyControlPlaneNamespace: "workspace-namespace",
			})
			testRun.Status.TestCases = append(testRun.Status.TestCases, v1alpha1.TestCaseStatus{
				Name:    createControlPlane,
				Exports: exportJSON,
			})

			err := createControlPlaneTest.Cleanup(testCtx, testRun, nil)

			Expect(err).NotTo(HaveOccurred())
		})

		It("should return error if deletion fails", func() {
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithInterceptorFuncs(interceptor.Funcs{
					Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
						return errors.NewServiceUnavailable("service unavailable")
					},
				}).
				Build()

			createControlPlaneTest = &CreateControlPlaneTest{OnboardingClient: fakeClient}
			exportJSON, _ := utils.MarshalToRawMessage(Exports{
				keyControlPlaneName:      "test-run-cp",
				keyControlPlaneNamespace: "workspace-namespace",
			})
			testRun.Status.TestCases = append(testRun.Status.TestCases, v1alpha1.TestCaseStatus{
				Name:    createControlPlane,
				Exports: exportJSON,
			})

			err := createControlPlaneTest.Cleanup(testCtx, testRun, nil)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("ControlPlane deletion failed"))
		})
	})
})
