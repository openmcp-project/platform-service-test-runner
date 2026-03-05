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

var _ = Describe("CreateMcpTest", func() {
	var (
		testCtx       context.Context
		scheme        *runtime.Scheme
		createMcpTest *CreateMcpTest
		testRun       *v1alpha1.E2ETestRun
		config        Config
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
		It("should create an MCPv2 and return exports", func() {
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
						if mcp, ok := obj.(*omcpv2alpha1.ManagedControlPlaneV2); ok && created {
							mcp.Status.Phase = common.StatusPhaseReady
						}
						return nil
					},
				}).
				Build()

			createMcpTest = &CreateMcpTest{OnboardingClient: fakeClient}
			exportJSON, _ := utils.MarshalToRawMessage(Exports{
				keyWorkspaceStatusNamespace: "workspace-namespace",
			})
			testRun.Status.TestCases = append(testRun.Status.TestCases, v1alpha1.TestCaseStatus{
				Name:    createWorkspace,
				Exports: exportJSON,
			})
			exports, _, err := createMcpTest.Run(testCtx, testRun, config)

			Expect(err).NotTo(HaveOccurred())
			Expect(exports[keyMcpName]).To(Equal("test-run-mcpv2"))
			Expect(exports[keyMcpNamespace]).To(Equal("workspace-namespace"))

			// Verify MCPv2 was created
			mcp := &omcpv2alpha1.ManagedControlPlaneV2{}
			Expect(fakeClient.Get(testCtx, client.ObjectKey{Name: "test-run-mcpv2", Namespace: "workspace-namespace"}, mcp)).To(Succeed())
			Expect(mcp.Labels["test-case"]).To(Equal(createMcpV2))
		})

		It("should return error if MCPv2 creation fails", func() {
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithInterceptorFuncs(interceptor.Funcs{
					Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
						return errors.NewBadRequest("creation failed")
					},
				}).
				Build()

			createMcpTest = &CreateMcpTest{OnboardingClient: fakeClient}
			exportJSON, _ := utils.MarshalToRawMessage(Exports{
				keyWorkspaceStatusNamespace: "workspace-namespace",
			})
			testRun.Status.TestCases = append(testRun.Status.TestCases, v1alpha1.TestCaseStatus{
				Name:    createWorkspace,
				Exports: exportJSON,
			})
			_, _, err := createMcpTest.Run(testCtx, testRun, config)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("creation failed"))
		})
	})

	Describe("Cleanup", func() {
		It("should delete MCPv2 successfully", func() {
			mcp := &omcpv2alpha1.ManagedControlPlaneV2{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-run-mcpv2",
					Namespace: "workspace-namespace",
				},
			}

			deleteCount := 0
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(mcp).
				WithInterceptorFuncs(interceptor.Funcs{
					Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
						if deleteCount > 0 {
							return errors.NewNotFound(schema.GroupResource{Group: "core.openmcp.cloud", Resource: "managedcontrolplanev2s"}, key.Name)
						}
						return c.Get(ctx, key, obj, opts...)
					},
					Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
						deleteCount++
						return c.Delete(ctx, obj, opts...)
					},
				}).
				Build()

			createMcpTest = &CreateMcpTest{OnboardingClient: fakeClient}
			exportJSON, _ := utils.MarshalToRawMessage(Exports{
				keyMcpName:      "test-run-mcpv2",
				keyMcpNamespace: "workspace-namespace",
			})
			testRun.Status.TestCases = append(testRun.Status.TestCases, v1alpha1.TestCaseStatus{
				Name:    createMcpV2,
				Exports: exportJSON,
			})

			err := createMcpTest.Cleanup(testCtx, testRun, nil)

			Expect(err).NotTo(HaveOccurred())
		})

		It("should succeed if MCPv2 already deleted", func() {
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithInterceptorFuncs(interceptor.Funcs{
					Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
						return errors.NewNotFound(schema.GroupResource{Group: "core.openmcp.cloud", Resource: "managedcontrolplanev2s"}, obj.GetName())
					},
				}).
				Build()

			createMcpTest = &CreateMcpTest{OnboardingClient: fakeClient}
			exportJSON, _ := utils.MarshalToRawMessage(Exports{
				keyMcpName:      "test-run-mcpv2",
				keyMcpNamespace: "workspace-namespace",
			})
			testRun.Status.TestCases = append(testRun.Status.TestCases, v1alpha1.TestCaseStatus{
				Name:    createMcpV2,
				Exports: exportJSON,
			})

			err := createMcpTest.Cleanup(testCtx, testRun, nil)

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

			createMcpTest = &CreateMcpTest{OnboardingClient: fakeClient}
			exportJSON, _ := utils.MarshalToRawMessage(Exports{
				keyMcpName:      "test-run-mcpv2",
				keyMcpNamespace: "workspace-namespace",
			})
			testRun.Status.TestCases = append(testRun.Status.TestCases, v1alpha1.TestCaseStatus{
				Name:    createMcpV2,
				Exports: exportJSON,
			})

			err := createMcpTest.Cleanup(testCtx, testRun, nil)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("MCPv2 deletion failed"))
		})
	})
})
