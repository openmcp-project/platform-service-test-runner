package runner

import (
	"context"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/openmcp-project/controller-utils/pkg/logging"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openmcp-project/platform-service-test-runner/api/v1alpha1"
	"github.com/openmcp-project/platform-service-test-runner/internal/utils"
)

const testManifest = `
apiVersion: crossplane.services.open-control-plane.io/v1alpha1
kind: Crossplane
spec:
  version: "1.15.0"
`

var _ = Describe("CreateServiceTest", func() {
	var (
		testCtx           context.Context
		scheme            *runtime.Scheme
		createServiceTest *CreateServiceTest
		testRun           *v1alpha1.E2ETestRun
		config            Config
		cpExportJSON      json.RawMessage
	)

	BeforeEach(func() {
		testCtx = logging.NewContext(context.Background(), logging.Discard())
		scheme = runtime.NewScheme()

		testRun = &v1alpha1.E2ETestRun{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-run",
			},
		}
		config = Config{
			"pollTimeout":  "1m",
			configManifest: testManifest,
		}

		var err error
		cpExportJSON, err = utils.MarshalToRawMessage(Exports{
			keyControlPlaneName:      "test-run-cp",
			keyControlPlaneNamespace: "cp-namespace",
		})
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("Run", func() {
		It("should create the service resource, poll assertions and return exports", func() {
			config[configAssertions] = []map[string]string{
				{"path": ".status.phase", "value": "Running"},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithInterceptorFuncs(interceptor.Funcs{
					Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
						u := obj.(*unstructured.Unstructured)
						u.Object["status"] = map[string]any{"phase": "Running"}
						return nil
					},
					Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
						u := obj.(*unstructured.Unstructured)
						u.SetName(key.Name)
						u.SetNamespace(key.Namespace)
						u.SetAPIVersion("crossplane.services.open-control-plane.io/v1alpha1")
						u.SetKind("Crossplane")
						u.Object["status"] = map[string]any{"phase": "Running"}
						return nil
					},
				}).
				Build()

			createServiceTest = &CreateServiceTest{OnboardingClient: fakeClient}
			testRun.Status.TestCases = append(testRun.Status.TestCases, v1alpha1.TestCaseStatus{
				Name:    createControlPlane,
				Exports: cpExportJSON,
			})

			exports, _, err := createServiceTest.Run(testCtx, testRun, config)

			Expect(err).NotTo(HaveOccurred())
			Expect(exports[keyServiceName]).To(Equal("test-run-cp"))
			Expect(exports[keyServiceNamespace]).To(Equal("cp-namespace"))
			Expect(exports[keyServiceKind]).To(Equal("Crossplane"))
		})

		It("should return error if createControlPlane status is missing", func() {
			createServiceTest = &CreateServiceTest{OnboardingClient: fake.NewClientBuilder().WithScheme(scheme).Build()}
			_, _, err := createServiceTest.Run(testCtx, testRun, config)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(createControlPlane))
		})

		It("should return error if manifest is missing from config", func() {
			delete(config, configManifest)
			createServiceTest = &CreateServiceTest{OnboardingClient: fake.NewClientBuilder().WithScheme(scheme).Build()}
			testRun.Status.TestCases = append(testRun.Status.TestCases, v1alpha1.TestCaseStatus{
				Name:    createControlPlane,
				Exports: cpExportJSON,
			})
			_, _, err := createServiceTest.Run(testCtx, testRun, config)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(configManifest))
		})

		It("should proceed if resource already exists", func() {
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithInterceptorFuncs(interceptor.Funcs{
					Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
						return errors.NewAlreadyExists(schema.GroupResource{Group: "crossplane.services.open-control-plane.io", Resource: "crossplanes"}, obj.GetName())
					},
					Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
						u := obj.(*unstructured.Unstructured)
						u.SetName(key.Name)
						u.SetNamespace(key.Namespace)
						u.SetAPIVersion("crossplane.services.open-control-plane.io/v1alpha1")
						u.SetKind("Crossplane")
						return nil
					},
				}).
				Build()

			createServiceTest = &CreateServiceTest{OnboardingClient: fakeClient}
			testRun.Status.TestCases = append(testRun.Status.TestCases, v1alpha1.TestCaseStatus{
				Name:    createControlPlane,
				Exports: cpExportJSON,
			})

			exports, _, err := createServiceTest.Run(testCtx, testRun, config)
			Expect(err).NotTo(HaveOccurred())
			Expect(exports[keyServiceName]).To(Equal("test-run-cp"))
		})
	})

	Describe("Cleanup", func() {
		It("should delete the service resource successfully", func() {
			deleteCount := 0
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithInterceptorFuncs(interceptor.Funcs{
					Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
						deleteCount++
						return nil
					},
					Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
						if deleteCount > 0 {
							return errors.NewNotFound(schema.GroupResource{}, key.Name)
						}
						return nil
					},
				}).
				Build()

			createServiceTest = &CreateServiceTest{OnboardingClient: fakeClient}
			svcExportJSON, err := utils.MarshalToRawMessage(Exports{
				keyServiceName:       "test-run-cp",
				keyServiceNamespace:  "cp-namespace",
				keyServiceAPIVersion: "crossplane.services.open-control-plane.io/v1alpha1",
				keyServiceKind:       "Crossplane",
			})
			Expect(err).NotTo(HaveOccurred())
			testRun.Status.TestCases = append(testRun.Status.TestCases, v1alpha1.TestCaseStatus{
				Name:    createService,
				Exports: svcExportJSON,
			})

			Expect(createServiceTest.Cleanup(testCtx, testRun, nil)).To(Succeed())
			Expect(deleteCount).To(Equal(1))
		})

		It("should succeed if resource already deleted", func() {
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithInterceptorFuncs(interceptor.Funcs{
					Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
						return errors.NewNotFound(schema.GroupResource{}, obj.GetName())
					},
				}).
				Build()

			createServiceTest = &CreateServiceTest{OnboardingClient: fakeClient}
			svcExportJSON, err := utils.MarshalToRawMessage(Exports{
				keyServiceName:       "test-run-cp",
				keyServiceNamespace:  "cp-namespace",
				keyServiceAPIVersion: "crossplane.services.open-control-plane.io/v1alpha1",
				keyServiceKind:       "Crossplane",
			})
			Expect(err).NotTo(HaveOccurred())
			testRun.Status.TestCases = append(testRun.Status.TestCases, v1alpha1.TestCaseStatus{
				Name:    createService,
				Exports: svcExportJSON,
			})

			Expect(createServiceTest.Cleanup(testCtx, testRun, nil)).To(Succeed())
		})
	})
})
