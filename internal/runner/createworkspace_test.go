package runner

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	pwv1alpha1 "github.com/openmcp-project/project-workspace-operator/api/core/v1alpha1"
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

var _ = Describe("CreateWorkspaceTest", func() {
	var (
		testCtx             context.Context
		scheme              *runtime.Scheme
		createWorkspaceTest *CreateWorkspaceTest
		testRun             *v1alpha1.E2ETestRun
		config              Config
	)

	BeforeEach(func() {
		testCtx = logging.NewContext(context.Background(), logging.Discard())
		scheme = runtime.NewScheme()
		Expect(pwv1alpha1.AddToScheme(scheme)).To(Succeed())
		Expect(v1alpha1.AddToScheme(scheme)).To(Succeed())

		testRun = &v1alpha1.E2ETestRun{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-run",
			},
		}
		config = Config{
			"identity": "test-user",
		}
	})

	Describe("Run", func() {
		It("should create a workspace and return exports", func() {
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
						// Simulate controller setting status.namespace after creation
						if ws, ok := obj.(*pwv1alpha1.Workspace); ok && created {
							ws.Status.Namespace = "workspace-namespace"
						}
						return nil
					},
				}).
				Build()

			createWorkspaceTest = &CreateWorkspaceTest{OnboardingClient: fakeClient}
			exportsJSON, _ := utils.MarshalToRawMessage(Exports{
				keyProjectStatusNamespace: "project-namespace",
			})
			testRun.Status.TestCases = append(testRun.Status.TestCases, v1alpha1.TestCaseStatus{
				Name:    createProject,
				Exports: exportsJSON,
			})
			exports, _, err := createWorkspaceTest.Run(testCtx, testRun, config)

			Expect(err).NotTo(HaveOccurred())
			Expect(exports[keyWorkspaceName]).To(Equal("test-run-ws"))
			Expect(exports[keyWorkspaceNamespace]).To(Equal("project-namespace"))
			Expect(exports[keyWorkspaceStatusNamespace]).To(Equal("workspace-namespace"))

			// Verify workspace was created
			workspace := &pwv1alpha1.Workspace{}
			Expect(fakeClient.Get(testCtx, client.ObjectKey{Name: "test-run-ws", Namespace: "project-namespace"}, workspace)).To(Succeed())
			Expect(workspace.Labels["test-case"]).To(Equal(createWorkspace))
		})

		It("should return error if workspace creation fails", func() {
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithInterceptorFuncs(interceptor.Funcs{
					Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
						return errors.NewBadRequest("creation failed")
					},
				}).
				Build()

			createWorkspaceTest = &CreateWorkspaceTest{OnboardingClient: fakeClient}
			exportJSON, _ := utils.MarshalToRawMessage(Exports{
				keyProjectStatusNamespace: "test-run-ws",
			})
			testRun.Status.TestCases = append(testRun.Status.TestCases, v1alpha1.TestCaseStatus{
				Name:    createProject,
				Exports: exportJSON,
			})
			_, _, err := createWorkspaceTest.Run(testCtx, testRun, config)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("workspace creation failed"))
		})
	})

	Describe("Cleanup", func() {
		It("should delete workspace successfully", func() {
			workspace := &pwv1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-run-ws",
					Namespace: "project-namespace",
				},
			}

			deleteCount := 0
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(workspace).
				WithInterceptorFuncs(interceptor.Funcs{
					Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
						if deleteCount > 0 {
							return errors.NewNotFound(schema.GroupResource{Group: "core.openmcp.cloud", Resource: "workspaces"}, key.Name)
						}
						return c.Get(ctx, key, obj, opts...)
					},
					Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
						deleteCount++
						return c.Delete(ctx, obj, opts...)
					},
				}).
				Build()

			createWorkspaceTest = &CreateWorkspaceTest{OnboardingClient: fakeClient}

			exportsJSON, _ := utils.MarshalToRawMessage(Exports{
				keyWorkspaceName:      "test-run-ws",
				keyWorkspaceNamespace: "project-namespace",
			})

			testRun.Status.TestCases = append(testRun.Status.TestCases, v1alpha1.TestCaseStatus{
				Name:    createWorkspace,
				Exports: exportsJSON,
			})

			err := createWorkspaceTest.Cleanup(testCtx, testRun, nil)

			Expect(err).NotTo(HaveOccurred())
		})

		It("should succeed if workspace already deleted", func() {
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithInterceptorFuncs(interceptor.Funcs{
					Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
						return errors.NewNotFound(schema.GroupResource{Group: "core.openmcp.cloud", Resource: "workspaces"}, obj.GetName())
					},
				}).
				Build()

			createWorkspaceTest = &CreateWorkspaceTest{OnboardingClient: fakeClient}

			exportsJSON, _ := utils.MarshalToRawMessage(Exports{
				keyWorkspaceName:      "non-existent-ws",
				keyWorkspaceNamespace: "project-namespace",
			})
			testRun.Status.TestCases = append(testRun.Status.TestCases, v1alpha1.TestCaseStatus{
				Name:    createWorkspace,
				Exports: exportsJSON,
			})

			err := createWorkspaceTest.Cleanup(testCtx, testRun, nil)

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

			createWorkspaceTest = &CreateWorkspaceTest{OnboardingClient: fakeClient}

			exportsJSON, _ := utils.MarshalToRawMessage(Exports{
				keyWorkspaceName:      "test-run-ws",
				keyWorkspaceNamespace: "project-namespace",
			})
			testRun.Status.TestCases = append(testRun.Status.TestCases, v1alpha1.TestCaseStatus{
				Name:    createWorkspace,
				Exports: exportsJSON,
			})

			err := createWorkspaceTest.Cleanup(testCtx, testRun, nil)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("workspace deletion failed"))
		})
	})
})
