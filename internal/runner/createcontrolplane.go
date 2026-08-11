package runner

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/openmcp-project/controller-utils/pkg/logging"
	"github.com/openmcp-project/openmcp-operator/api/common"
	omcpv2alpha1 "github.com/openmcp-project/openmcp-operator/api/core/v2alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openmcp-project/platform-service-test-runner/internal/utils"

	"github.com/openmcp-project/platform-service-test-runner/api/v1alpha1"
)

const (
	createControlPlane       = "createControlPlane"
	keyControlPlaneName      = "controlplane.name"
	keyControlPlaneNamespace = "controlplane.namespace"
)

// CreateControlPlaneTest implements the TestCase interface for creating a ControlPlane resource.
type CreateControlPlaneTest struct {
	OnboardingClient client.Client
}

// StatusName returns the fixed status name for this test case.
func (c *CreateControlPlaneTest) StatusName(_ Config) string { return createControlPlane }

// Run creates a ControlPlane in the workspace created by the createWorkspace test case, with the given configuration, and waits until it's ready.
// It returns the ControlPlane name and namespace as exports for other test cases to use.
func (c *CreateControlPlaneTest) Run(ctx context.Context, run *v1alpha1.E2ETestRun, config Config) (Exports, DebugInfo, error) {
	ctxTimeout := GetContextTimeoutOrDefault(config)
	log := logging.FromContextOrPanic(ctx).WithName(createControlPlane)
	ctx, cancel := context.WithTimeout(ctx, ctxTimeout)
	defer cancel()

	// ControlPlane creation depends on workspace creation, so we need to get the workspace namespace from the previous test case's exports
	wsTest, found := GetStatus(createWorkspace, run.Status.TestCases)
	if !found {
		return nil, nil, fmt.Errorf("dependent test case %s has no status", createWorkspace)
	}

	cpName := fmt.Sprintf("%s-cp", run.Name)

	// Unmarshal exports from JSON
	var exports Exports
	if err := json.Unmarshal(wsTest.Exports, &exports); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal exports from %s: %w", createWorkspace, err)
	}

	cpNamespace := utils.GetAsString(exports, keyWorkspaceStatusNamespace)

	cp := &omcpv2alpha1.ControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cpName,
			Namespace: cpNamespace,
			Labels: map[string]string{
				labelTestCase: createControlPlane,
			},
		},
		Spec: omcpv2alpha1.ControlPlaneSpec{},
	}

	if err := c.OnboardingClient.Create(ctx, cp); err != nil {
		if !errors.IsAlreadyExists(err) {
			return nil, nil, fmt.Errorf("ControlPlane creation failed: %w", err)
		}
		log.Info("ControlPlane already exists, proceeding with existing resource", keyControlPlaneName, cp.Name, keyControlPlaneNamespace, cp.Namespace)
	} else {
		log.Info("ControlPlane created", keyControlPlaneName, cp.Name, keyControlPlaneNamespace, cp.Namespace)
	}

	pollInterval := GetPollIntervalOrDefault(config)
	pollTimeout := GetPollTimeoutOrDefault(config)

	cp, err := WaitForReadyAndGet(ctx, c.OnboardingClient, cpName, cpNamespace, cp, pollInterval, pollTimeout, IsControlPlaneReady)
	if err != nil {
		return nil, nil, fmt.Errorf("polling ControlPlane after creation: %w", err)
	}
	log.Info("ControlPlane ready", keyControlPlaneName, cp.Name, keyControlPlaneNamespace, cp.Namespace)

	return Exports{
		keyControlPlaneName:      cp.Name,
		keyControlPlaneNamespace: cp.Namespace,
	}, nil, nil
}

// Cleanup deletes the ControlPlane created in the Run method, identified via own export. It waits until the ControlPlane is fully deleted before returning.
func (c *CreateControlPlaneTest) Cleanup(ctx context.Context, run *v1alpha1.E2ETestRun, config Config) error {
	log := logging.FromContextOrPanic(ctx).WithName(createControlPlane)
	ctx, cancel := context.WithTimeout(ctx, defaultContextTimeout)
	defer cancel()

	ownStatus, found := GetStatus(createControlPlane, run.Status.TestCases)
	if !found {
		return fmt.Errorf("cannot find '%s' test case status", createControlPlane)
	}

	// Unmarshal exports from JSON
	var exports Exports
	if err := json.Unmarshal(ownStatus.Exports, &exports); err != nil {
		return fmt.Errorf("failed to unmarshal exports from %s: %w", createControlPlane, err)
	}

	cpName := utils.GetAsString(exports, keyControlPlaneName)
	if cpName == "" {
		return fmt.Errorf("export %s not found or empty", keyControlPlaneName)
	}

	cpNs := utils.GetAsString(exports, keyControlPlaneNamespace)
	if cpNs == "" {
		return fmt.Errorf("export %s not found or empty", keyControlPlaneNamespace)
	}

	cp := &omcpv2alpha1.ControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cpName,
			Namespace: cpNs,
		},
	}

	log.Debug("Deleting ControlPlane", keyControlPlaneName, cpName, keyControlPlaneNamespace, cpNs)
	if err := c.OnboardingClient.Delete(ctx, cp); err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("ControlPlane deletion failed: %w", err)
		}
		return nil
	}

	pollInterval := GetPollIntervalOrDefault(config)
	pollTimeout := GetPollTimeoutOrDefault(config)
	err := WaitForDeletion(ctx, c.OnboardingClient, cpName, cpNs, &omcpv2alpha1.ControlPlane{}, pollInterval, pollTimeout)
	if err != nil {
		return fmt.Errorf("polling ControlPlane after deletion failed: %w", err)
	}
	log.Debug("ControlPlane deleted", keyControlPlaneName, cp.Name, keyControlPlaneNamespace, cp.Namespace)

	return nil
}

// IsControlPlaneReady checks if a ControlPlane is ready by verifying its status phase.
func IsControlPlaneReady(cp *omcpv2alpha1.ControlPlane) bool {
	return cp.Status.Phase == common.StatusPhaseReady
}
