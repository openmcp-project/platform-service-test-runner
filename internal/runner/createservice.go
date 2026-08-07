package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/openmcp-project/controller-utils/pkg/logging"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/util/jsonpath"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/openmcp-project/platform-service-test-runner/api/v1alpha1"
	"github.com/openmcp-project/platform-service-test-runner/internal/utils"
)

const (
	createService        = "createService"
	configManifest       = "manifest"
	configAssertions     = "assertions"
	keyServiceName       = "service.name"
	keyServiceNamespace  = "service.namespace"
	keyServiceAPIVersion = "service.apiVersion"
	keyServiceKind       = "service.kind"
)

// JSONPathAssertion is a single JSONPath assertion against the service resource.
type JSONPathAssertion struct {
	Path  string `json:"path"`
	Value string `json:"value"`
}

type CreateServiceTest struct {
	OnboardingClient client.Client
}

// StatusName returns "createService/<Kind>" derived from the manifest in config,
// so that multiple createService entries with different service kinds get distinct status names.
func (c *CreateServiceTest) StatusName(config Config) string {
	manifestYAML := utils.GetAsString(config, configManifest)
	if manifestYAML == "" {
		return createService
	}
	obj, err := parseManifest(manifestYAML)
	if err != nil || obj.GetKind() == "" {
		return createService
	}
	return createService + "/" + obj.GetKind()
}

// Run applies an arbitrary ServiceProvider resource derived from the manifest in config to the
// onboarding cluster in the namespace of the ControlPlane created by createControlPlane. It then
// polls until all JSONPath assertions pass.
func (c *CreateServiceTest) Run(ctx context.Context, run *v1alpha1.E2ETestRun, config Config) (Exports, DebugInfo, error) {
	ctxTimeout := GetContextTimeoutOrDefault(config)
	log := logging.FromContextOrPanic(ctx).WithName(createService)
	ctx, cancel := context.WithTimeout(ctx, ctxTimeout)
	defer cancel()

	cpStatus, found := GetStatus(createControlPlane, run.Status.TestCases)
	if !found {
		return nil, nil, fmt.Errorf("dependent test case %s has no status", createControlPlane)
	}

	var cpExports Exports
	if err := json.Unmarshal(cpStatus.Exports, &cpExports); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal exports from %s: %w", createControlPlane, err)
	}

	cpName := utils.GetAsString(cpExports, keyControlPlaneName)
	cpNamespace := utils.GetAsString(cpExports, keyControlPlaneNamespace)
	if cpName == "" || cpNamespace == "" {
		return nil, nil, fmt.Errorf("exports %s and %s must be set by %s", keyControlPlaneName, keyControlPlaneNamespace, createControlPlane)
	}

	manifestYAML := utils.GetAsString(config, configManifest)
	if manifestYAML == "" {
		return nil, nil, fmt.Errorf("config key %q is required", configManifest)
	}

	obj, err := parseManifest(manifestYAML)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse manifest: %w", err)
	}

	obj.SetName(cpName)
	obj.SetNamespace(cpNamespace)

	assertions, err := parseAssertions(config)
	if err != nil {
		return nil, nil, err
	}

	if err := c.OnboardingClient.Create(ctx, obj); err != nil {
		if !errors.IsAlreadyExists(err) {
			return nil, nil, fmt.Errorf("service resource creation failed: %w", err)
		}
		log.Info("Service resource already exists, proceeding with existing resource", keyServiceName, obj.GetName(), keyServiceNamespace, obj.GetNamespace())
	} else {
		log.Info("Service resource created", keyServiceName, obj.GetName(), keyServiceNamespace, obj.GetNamespace())
	}

	pollInterval := GetPollIntervalOrDefault(config)
	pollTimeout := GetPollTimeoutOrDefault(config)

	obj, err = WaitForReadyAndGet(ctx, c.OnboardingClient, cpName, cpNamespace, obj, pollInterval, pollTimeout, func(o *unstructured.Unstructured) bool {
		return assertionsPass(o, assertions)
	})
	if err != nil {
		return nil, nil, fmt.Errorf("polling service resource after creation: %w", err)
	}
	log.Info("Service resource ready", keyServiceName, obj.GetName(), keyServiceNamespace, obj.GetNamespace())

	return Exports{
		keyServiceName:       obj.GetName(),
		keyServiceNamespace:  obj.GetNamespace(),
		keyServiceAPIVersion: obj.GetAPIVersion(),
		keyServiceKind:       obj.GetKind(),
	}, nil, nil
}

// Cleanup deletes the service resource created in Run, identified via own exports.
func (c *CreateServiceTest) Cleanup(ctx context.Context, run *v1alpha1.E2ETestRun, config Config) error {
	log := logging.FromContextOrPanic(ctx).WithName(createService)
	ctx, cancel := context.WithTimeout(ctx, defaultContextTimeout)
	defer cancel()

	ownStatus, found := GetStatus(c.StatusName(config), run.Status.TestCases)
	if !found {
		return fmt.Errorf("cannot find '%s' test case status", c.StatusName(config))
	}

	var exports Exports
	if err := json.Unmarshal(ownStatus.Exports, &exports); err != nil {
		return fmt.Errorf("failed to unmarshal exports from %s: %w", createService, err)
	}

	svcName := utils.GetAsString(exports, keyServiceName)
	if svcName == "" {
		return fmt.Errorf("export %s not found or empty", keyServiceName)
	}
	svcNamespace := utils.GetAsString(exports, keyServiceNamespace)
	if svcNamespace == "" {
		return fmt.Errorf("export %s not found or empty", keyServiceNamespace)
	}
	apiVersion := utils.GetAsString(exports, keyServiceAPIVersion)
	kind := utils.GetAsString(exports, keyServiceKind)

	obj := &unstructured.Unstructured{}
	obj.SetName(svcName)
	obj.SetNamespace(svcNamespace)
	obj.SetAPIVersion(apiVersion)
	obj.SetKind(kind)

	log.Debug("Deleting service resource", keyServiceName, svcName, keyServiceNamespace, svcNamespace)
	if err := c.OnboardingClient.Delete(ctx, obj); err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("service resource deletion failed: %w", err)
		}
		return nil
	}

	pollInterval := GetPollIntervalOrDefault(config)
	pollTimeout := GetPollTimeoutOrDefault(config)
	if err := WaitForDeletion(ctx, c.OnboardingClient, svcName, svcNamespace, &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": apiVersion,
			"kind":       kind,
		},
	}, pollInterval, pollTimeout); err != nil {
		return fmt.Errorf("polling service resource after deletion failed: %w", err)
	}
	log.Debug("Service resource deleted", keyServiceName, svcName, keyServiceNamespace, svcNamespace)

	return nil
}

func parseManifest(manifestYAML string) (*unstructured.Unstructured, error) {
	jsonBytes, err := yaml.YAMLToJSON([]byte(manifestYAML))
	if err != nil {
		return nil, err
	}
	var raw map[string]any
	if err := json.Unmarshal(jsonBytes, &raw); err != nil {
		return nil, err
	}
	return &unstructured.Unstructured{Object: raw}, nil
}

func parseAssertions(config Config) ([]JSONPathAssertion, error) {
	raw, ok := config[configAssertions]
	if !ok {
		return nil, nil
	}
	jsonBytes, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal assertions config: %w", err)
	}
	var assertions []JSONPathAssertion
	if err := json.Unmarshal(jsonBytes, &assertions); err != nil {
		return nil, fmt.Errorf("failed to parse assertions: %w", err)
	}
	return assertions, nil
}

func assertionsPass(obj *unstructured.Unstructured, assertions []JSONPathAssertion) bool {
	for _, a := range assertions {
		jp := jsonpath.New("assertion").AllowMissingKeys(false)
		if err := jp.Parse(fmt.Sprintf("{%s}", a.Path)); err != nil {
			return false
		}
		var buf bytes.Buffer
		if err := jp.Execute(&buf, obj.Object); err != nil {
			return false
		}
		if buf.String() != a.Value {
			return false
		}
	}
	return true
}
