package runner

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/openmcp-project/controller-utils/pkg/logging"
	pwv1alpha1 "github.com/openmcp-project/project-workspace-operator/api/core/v1alpha1"
	v1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openmcp-project/platform-service-test-runner/internal/utils"

	"github.com/openmcp-project/platform-service-test-runner/api/v1alpha1"
)

const (
	createWorkspace             = "createWorkspace"
	keyWorkspaceName            = "workspace.name"
	keyWorkspaceNamespace       = "workspace.namespace"
	keyWorkspaceStatusNamespace = "workspace.status.namespace"
)

type CreateWorkspaceTest struct {
	OnboardingClient client.Client
}

// Run creates a workspace in the project created by the createProject test case, with the given configuration, and waits until it's ready.
// It returns the workspace name, namespace, and status namespace as exports for other test cases to use.
func (c *CreateWorkspaceTest) Run(ctx context.Context, run *v1alpha1.E2ETestRun, config Config) (Exports, DebugInfo, error) {
	log := logging.FromContextOrPanic(ctx).WithName(createWorkspace)
	ctx, cancel := context.WithTimeout(ctx, defaultContextTimeout)
	defer cancel()

	// Workspace creation depends on project creation, so we need to get the project namespace from the previous test case's exports
	pTest, found := GetStatus(createProject, run.Status.TestCases)
	if !found {
		return nil, nil, fmt.Errorf("dependent test case %s has no status", createProject)
	}

	wsName := fmt.Sprintf("%s-ws", run.Name)

	// Unmarshal exports from JSON
	var exports Exports
	if err := json.Unmarshal(pTest.Exports, &exports); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal exports from %s: %w", createProject, err)
	}

	wsNamespace := utils.GetAsString(exports, keyProjectStatusNamespace)
	if wsNamespace == "" {
		return nil, nil, fmt.Errorf("export %s not found or empty", keyWorkspaceNamespace)
	}
	identity := utils.GetAsString(config, configIdentity)
	if identity == "" {
		return nil, nil, fmt.Errorf("config %s not found or empty", configIdentity)
	}

	members := []pwv1alpha1.WorkspaceMember{
		{
			Subject: pwv1alpha1.Subject{
				Kind: v1.UserKind,
				Name: identity,
			},
			Roles: []pwv1alpha1.WorkspaceMemberRole{
				pwv1alpha1.WorkspaceRoleAdmin,
			},
		},
	}

	workspace := &pwv1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      wsName,
			Namespace: wsNamespace,
			Labels: map[string]string{
				labelTestCase: createWorkspace,
			},
		},
		Spec: pwv1alpha1.WorkspaceSpec{
			Members: members,
		},
	}

	if err := c.OnboardingClient.Create(ctx, workspace); err != nil {
		return nil, nil, fmt.Errorf("workspace creation failed: %w", err)
	}
	log.Debug("Workspace created", keyWorkspaceName, workspace.Name, keyWorkspaceNamespace, workspace.Namespace)

	pollInterval := GetPollIntervalOrDefault(config)
	pollTimeout := GetPollTimeoutOrDefault(config)
	workspace, err := WaitForReadyAndGet(ctx, c.OnboardingClient, wsName, wsNamespace, workspace, pollInterval, pollTimeout, IsWorkspaceReady)
	if err != nil {
		return nil, nil, fmt.Errorf("polling workspace after creation failed: %w", err)
	}
	log.Debug("Workspace ready", keyWorkspaceName, workspace.Name, keyWorkspaceNamespace, workspace.Namespace, keyWorkspaceStatusNamespace, workspace.Status.Namespace)

	return Exports{
		keyWorkspaceName:            workspace.Name,
		keyWorkspaceNamespace:       workspace.Namespace,
		keyWorkspaceStatusNamespace: workspace.Status.Namespace,
	}, nil, nil
}

// Cleanup deletes the workspace created in the Run method, identified via own export. It waits until the workspace is fully deleted before returning.
func (c *CreateWorkspaceTest) Cleanup(ctx context.Context, run *v1alpha1.E2ETestRun, config Config) error {
	log := logging.FromContextOrPanic(ctx).WithName(createWorkspace)
	ctx, cancel := context.WithTimeout(ctx, defaultContextTimeout)
	defer cancel()

	ownStatus, found := GetStatus(createWorkspace, run.Status.TestCases)
	if !found {
		return fmt.Errorf("cannot find '%s' test case status", createWorkspace)
	}

	// Unmarshal exports from JSON
	var exports Exports
	if err := json.Unmarshal(ownStatus.Exports, &exports); err != nil {
		return fmt.Errorf("failed to unmarshal exports from %s: %w", createWorkspace, err)
	}

	wsName := utils.GetAsString(exports, keyWorkspaceName)
	if wsName == "" {
		return fmt.Errorf("export %s not found or empty", keyWorkspaceName)
	}
	wsNamespace := utils.GetAsString(exports, keyWorkspaceNamespace)
	if wsNamespace == "" {
		return fmt.Errorf("export %s not found or empty", keyWorkspaceNamespace)
	}

	ws := &pwv1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      wsName,
			Namespace: wsNamespace,
		},
	}

	if err := c.OnboardingClient.Delete(ctx, ws); err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("workspace deletion failed: %w", err)
		}
		return nil // Already gone
	}

	pollInterval := GetPollIntervalOrDefault(config)
	pollTimeout := GetPollTimeoutOrDefault(config)
	err := WaitForDeletion(ctx, c.OnboardingClient, wsName, wsNamespace, &pwv1alpha1.Workspace{}, pollInterval, pollTimeout)
	if err != nil {
		return fmt.Errorf("polling after workspace deletion failed: %w", err)
	}
	log.Info("Workspace deleted", keyWorkspaceName, ws.Name, keyWorkspaceNamespace, ws.Namespace)
	return nil
}

// IsWorkspaceReady checks if a Workspace is ready by verifying its status namespace is set.
func IsWorkspaceReady(ws *pwv1alpha1.Workspace) bool {
	return ws.Status.Namespace != ""
}
