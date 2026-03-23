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
	createMcpV2     = "createManagedControlPlaneV2"
	keyMcpName      = "mcp.name"
	keyMcpNamespace = "mcp.namespace"
)

type CreateMcpTest struct {
	OnboardingClient client.Client
}

// Run creates a MCPv2 in the workspace created by the createWorkspace test case, with the given configuration, and waits until it's ready.
// It returns the MCPv2 name and namespace as exports for other test cases to use.
func (c *CreateMcpTest) Run(ctx context.Context, run *v1alpha1.E2ETestRun, config Config) (Exports, DebugInfo, error) {
	ctxTimeout := GetContextTimeoutOrDefault(config)
	log := logging.FromContextOrPanic(ctx).WithName(createMcpV2)
	ctx, cancel := context.WithTimeout(ctx, ctxTimeout)
	defer cancel()

	// mcp creation depends on workspace creation, so we need to get the workspace namespace from the previous test case's exports
	wsTest, found := GetStatus(createWorkspace, run.Status.TestCases)
	if !found {
		return nil, nil, fmt.Errorf("dependent test case %s has no status", createWorkspace)
	}

	mcpName := fmt.Sprintf("%s-mcpv2", run.Name)

	// Unmarshal exports from JSON
	var exports Exports
	if err := json.Unmarshal(wsTest.Exports, &exports); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal exports from %s: %w", createWorkspace, err)
	}

	mcpNamespace := utils.GetAsString(exports, keyWorkspaceStatusNamespace)

	mcp := &omcpv2alpha1.ManagedControlPlaneV2{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mcpName,
			Namespace: mcpNamespace,
			Labels: map[string]string{
				labelTestCase: createMcpV2,
			},
		},
		Spec: omcpv2alpha1.ManagedControlPlaneV2Spec{},
	}

	if err := c.OnboardingClient.Create(ctx, mcp); err != nil {
		if !errors.IsAlreadyExists(err) {
			return nil, nil, fmt.Errorf("MCPv2 creation failed: %w", err)
		}
		log.Info("MCPv2 already exists, proceeding with existing resource", keyMcpName, mcp.Name, keyMcpNamespace, mcp.Namespace)
	} else {
		log.Info("MCPv2 created", keyMcpName, mcp.Name, keyMcpNamespace, mcp.Namespace)
	}

	pollInterval := GetPollIntervalOrDefault(config)
	pollTimeout := GetPollTimeoutOrDefault(config)

	mcp, err := WaitForReadyAndGet(ctx, c.OnboardingClient, mcpName, mcpNamespace, mcp, pollInterval, pollTimeout, IsMcpReady)
	if err != nil {
		return nil, nil, fmt.Errorf("polling MCPv2 after creation: %w", err)
	}
	log.Info("MCPv2 ready", keyMcpName, mcp.Name, keyMcpNamespace, mcp.Namespace)

	return Exports{
		keyMcpName:      mcp.Name,
		keyMcpNamespace: mcp.Namespace,
	}, nil, nil
}

// Cleanup deletes the MCPv2 created in the Run method, identified via own export. It waits until the MCPv2 is fully deleted before returning.
func (c *CreateMcpTest) Cleanup(ctx context.Context, run *v1alpha1.E2ETestRun, config Config) error {
	log := logging.FromContextOrPanic(ctx).WithName(createMcpV2)
	ctx, cancel := context.WithTimeout(ctx, defaultContextTimeout)
	defer cancel()

	ownStatus, found := GetStatus(createMcpV2, run.Status.TestCases)
	if !found {
		return fmt.Errorf("cannot find '%s' test case status", createMcpV2)
	}

	// Unmarshal exports from JSON
	var exports Exports
	if err := json.Unmarshal(ownStatus.Exports, &exports); err != nil {
		return fmt.Errorf("failed to unmarshal exports from %s: %w", createMcpV2, err)
	}

	mcpName := utils.GetAsString(exports, keyMcpName)
	if mcpName == "" {
		return fmt.Errorf("export %s not found or empty", keyMcpName)
	}

	mcpNs := utils.GetAsString(exports, keyMcpNamespace)
	if mcpNs == "" {
		return fmt.Errorf("export %s not found or empty", keyMcpNamespace)
	}

	mcp := &omcpv2alpha1.ManagedControlPlaneV2{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mcpName,
			Namespace: mcpNs,
		},
	}

	log.Debug("Deleting MCPv2", keyMcpName, mcpName, keyMcpNamespace, mcp.Namespace)
	if err := c.OnboardingClient.Delete(ctx, mcp); err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("MCPv2 deletion failed: %w", err)
		}
		return nil
	}

	pollInterval := GetPollIntervalOrDefault(config)
	pollTimeout := GetPollTimeoutOrDefault(config)
	err := WaitForDeletion(ctx, c.OnboardingClient, mcpName, mcpNs, &omcpv2alpha1.ManagedControlPlaneV2{}, pollInterval, pollTimeout)
	if err != nil {
		return fmt.Errorf("polling MCPv2 after deletion failed: %w", err)
	}
	log.Debug("MCPv2 deleted", keyMcpName, mcp.Name, keyMcpNamespace, mcp.Namespace)

	return nil
}

// IsMcpReady checks if a ManagedControlPlaneV2 is ready by verifying its status phase.
func IsMcpReady(mcp *omcpv2alpha1.ManagedControlPlaneV2) bool {
	return mcp.Status.Phase == common.StatusPhaseReady
}
