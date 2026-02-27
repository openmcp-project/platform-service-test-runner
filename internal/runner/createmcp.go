package runner

import (
	"context"
	"fmt"
	"time"

	"github.com/openmcp-project/controller-utils/pkg/logging"
	"github.com/openmcp-project/openmcp-operator/api/common"
	omcpv2alpha1 "github.com/openmcp-project/openmcp-operator/api/core/v2alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

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
func (c *CreateMcpTest) Run(ctx context.Context, run *v1alpha1.E2ETestRun, _ map[string]string) (map[string]string, map[string]string, error) {
	log := logging.FromContextOrPanic(ctx).WithName(createMcpV2)
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()

	// mcp creation depends on workspace creation, so we need to get the workspace namespace from the previous test case's exports
	wsTest, found := GetStatus(createWorkspace, run.Status.TestCases)
	if !found {
		return nil, nil, fmt.Errorf("dependent test case %s has no status", createWorkspace)
	}

	mcpName := fmt.Sprintf("%s-mcpv2", run.Name)
	ns, found := wsTest.Exports[keyWorkspaceStatusNamespace]
	if !found {
		return nil, nil, fmt.Errorf("dependent test case %s has no export %s", createWorkspace, keyWorkspaceStatusNamespace)
	}

	mcp := &omcpv2alpha1.ManagedControlPlaneV2{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mcpName,
			Namespace: ns,
			Labels: map[string]string{
				labelTestCase: createMcpV2,
			},
		},
		Spec: omcpv2alpha1.ManagedControlPlaneV2Spec{},
	}

	if err := c.OnboardingClient.Create(ctx, mcp); err != nil {
		return nil, nil, err
	}
	log.Info("MCPv2 created", keyMcpName, mcp.Name, keyMcpNamespace, mcp.Namespace)

	mcp, err := WaitForReadyAndGet(ctx, c.OnboardingClient, mcpName, ns, mcp, IsMcpReady)
	if err != nil {
		return nil, nil, fmt.Errorf("polling MCPv2 after creation: %w", err)
	}
	log.Info("MCPv2 ready", keyMcpName, mcp.Name, keyMcpNamespace, mcp.Namespace)

	return map[string]string{
		keyMcpName:      mcp.Name,
		keyMcpNamespace: mcp.Namespace,
	}, nil, nil
}

// Cleanup deletes the MCPv2 created in the Run method, identified via own export. It waits until the MCPv2 is fully deleted before returning.
func (c *CreateMcpTest) Cleanup(ctx context.Context, run *v1alpha1.E2ETestRun, _ map[string]string) error {
	log := logging.FromContextOrPanic(ctx).WithName(createMcpV2)
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()

	ownStatus, found := GetStatus(createMcpV2, run.Status.TestCases)
	if !found {
		return fmt.Errorf("cannot find '%s' test case status", createMcpV2)
	}

	mcpName, mcpNs := ownStatus.Exports[keyMcpName], ownStatus.Exports[keyMcpNamespace]

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

	// Wait for deletion to complete
	err := WaitForDeletion(ctx, c.OnboardingClient, mcpName, mcpNs, &omcpv2alpha1.ManagedControlPlaneV2{})
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
