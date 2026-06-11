package runner

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	pwv1alpha1 "github.com/openmcp-project/platform-service-project-workspace/api/v2/core/v1alpha1"
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

var _ = Describe("CreateProjectTest", func() {
	var (
		testCtx           context.Context
		scheme            *runtime.Scheme
		createProjectTest *CreateProjectTest
		testRun           *v1alpha1.E2ETestRun
		config            Config
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
			"identity":           "test-user",
			"chargingTargetType": "cost-center",
			"chargingTarget":     "12345",
		}
	})

	Describe("Run", func() {
		It("should create a project and return exports", func() {
			// Mock client that returns status.namespace on Get after Create
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
						if p, ok := obj.(*pwv1alpha1.Project); ok && created {
							p.Status.Namespace = "project-namespace"
						}
						return nil
					},
				}).
				Build()

			createProjectTest = &CreateProjectTest{OnboardingClient: fakeClient}

			exports, _, err := createProjectTest.Run(testCtx, testRun, config)

			Expect(err).NotTo(HaveOccurred())
			Expect(exports[keyProjectName]).To(Equal("test-run-p"))
			Expect(exports[keyProjectStatusNamespace]).To(Equal("project-namespace"))

			// Verify project was created
			project := &pwv1alpha1.Project{}
			Expect(fakeClient.Get(testCtx, client.ObjectKey{Name: "test-run-p"}, project)).To(Succeed())
			Expect(project.Labels["test-case"]).To(Equal(createProject))
		})

		It("should poll state if project already exists and return exports", func() {
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithInterceptorFuncs(interceptor.Funcs{
					Create: func(ctx context.Context, client client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
						return errors.NewAlreadyExists(schema.GroupResource{Group: "core.openmcp.cloud", Resource: "projects"}, obj.GetName())
					},
					Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
						// Simulate controller setting status.namespace to indicate project is ready
						if p, ok := obj.(*pwv1alpha1.Project); ok {
							p.Status.Namespace = "project-namespace"
							return nil
						}
						return c.Get(ctx, key, obj, opts...)
					},
				}).
				Build()

			createProjectTest = &CreateProjectTest{OnboardingClient: fakeClient}

			exports, _, err := createProjectTest.Run(testCtx, testRun, config)

			Expect(err).NotTo(HaveOccurred())
			Expect(exports[keyProjectName]).To(Equal("test-run-p"))
			Expect(exports[keyProjectStatusNamespace]).To(Equal("project-namespace"))
		})

		It("should return error if project creation fails", func() {
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithInterceptorFuncs(interceptor.Funcs{
					Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
						return errors.NewBadRequest("creation failed")
					},
				}).
				Build()

			createProjectTest = &CreateProjectTest{OnboardingClient: fakeClient}

			_, _, err := createProjectTest.Run(testCtx, testRun, config)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("project creation failed"))
		})
	})

	Describe("Cleanup", func() {
		It("should delete project successfully", func() {
			// Create project first
			project := &pwv1alpha1.Project{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-run-p",
				},
			}

			deleteCount := 0
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(project).
				WithInterceptorFuncs(interceptor.Funcs{
					Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
						// After delete, return NotFound
						if deleteCount > 0 {
							return errors.NewNotFound(schema.GroupResource{Group: "core.openmcp.cloud", Resource: "projects"}, key.Name)
						}
						return c.Get(ctx, key, obj, opts...)
					},
					Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
						deleteCount++
						return c.Delete(ctx, obj, opts...)
					},
				}).
				Build()

			createProjectTest = &CreateProjectTest{OnboardingClient: fakeClient}

			exportsJSON, _ := utils.MarshalToRawMessage(Exports{
				keyProjectName: "test-run-p",
			})
			testRun.Status.TestCases = append(testRun.Status.TestCases, v1alpha1.TestCaseStatus{
				Name:    createProject,
				Exports: exportsJSON,
			})

			err := createProjectTest.Cleanup(testCtx, testRun, nil)

			Expect(err).NotTo(HaveOccurred())
		})

		It("should succeed if project already deleted", func() {
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithInterceptorFuncs(interceptor.Funcs{
					Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
						return errors.NewNotFound(schema.GroupResource{Group: "core.openmcp.cloud", Resource: "projects"}, obj.GetName())
					},
				}).
				Build()

			createProjectTest = &CreateProjectTest{OnboardingClient: fakeClient}

			exportsJSON, _ := utils.MarshalToRawMessage(Exports{
				keyProjectName: "non-existent-project",
			})
			testRun.Status.TestCases = append(testRun.Status.TestCases, v1alpha1.TestCaseStatus{
				Name:    createProject,
				Exports: exportsJSON,
			})

			err := createProjectTest.Cleanup(testCtx, testRun, nil)

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

			createProjectTest = &CreateProjectTest{OnboardingClient: fakeClient}

			exportsJSON, _ := utils.MarshalToRawMessage(Exports{
				keyProjectName: "test-run-p",
			})
			testRun.Status.TestCases = append(testRun.Status.TestCases, v1alpha1.TestCaseStatus{
				Name:    createProject,
				Exports: exportsJSON,
			})

			err := createProjectTest.Cleanup(testCtx, testRun, nil)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("project deletion failed"))
		})
	})
})
