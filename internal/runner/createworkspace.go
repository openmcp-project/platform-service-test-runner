package runner

import (
	"context"
	"fmt"
	"time"

	"github.com/openmcp-project/controller-utils/pkg/logging"
	pwv1alpha1 "github.com/openmcp-project/project-workspace-operator/api/core/v1alpha1"
	v1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openmcp-project/platform-service-test-runner/internal/util"

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
func (c *CreateWorkspaceTest) Run(ctx context.Context, run *v1alpha1.E2ETestRun, config map[string]string) (map[string]string, map[string]string, error) {
	log := logging.FromContextOrPanic(ctx).WithName(createWorkspace)
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()

	// worspace creation depends on project creation, so we need to get the project namespace from the previous test case's exports
	pTest, found := util.GetStatus(createProject, run.Status.TestCases)
	if !found {
		return nil, nil, fmt.Errorf("dependent test case %s has no status", createProject)
	}

	wsName := fmt.Sprintf("%s-ws", run.Name)
	wsNamespace, found := pTest.Exports[keyProjectStatusNamespace]
	if !found {
		return nil, nil, fmt.Errorf("dependent test case %s has no export %s", createProject, keyProjectStatusNamespace)
	}

	members := []pwv1alpha1.WorkspaceMember{
		{
			Subject: pwv1alpha1.Subject{
				Kind: v1.UserKind,
				Name: config[configIdentity],
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

	workspace, err := c.waitForReadyAndGet(ctx, wsName, wsNamespace)
	if err != nil {
		return nil, nil, fmt.Errorf("polling workspace after creation failed: %w", err)
	}
	log.Debug("Workspace ready", keyWorkspaceName, workspace.Name, keyWorkspaceNamespace, workspace.Namespace, keyWorkspaceStatusNamespace, workspace.Status.Namespace)

	return map[string]string{
		keyWorkspaceName:            workspace.Name,
		keyWorkspaceNamespace:       workspace.Namespace,
		keyWorkspaceStatusNamespace: workspace.Status.Namespace,
	}, nil, nil
}

// Cleanup deletes the workspace created in the Run method, identified via own export. It waits until the workspace is fully deleted before returning.
func (c *CreateWorkspaceTest) Cleanup(ctx context.Context, run *v1alpha1.E2ETestRun, _ map[string]string) error {
	log := logging.FromContextOrPanic(ctx).WithName(createWorkspace)
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()

	ownStatus, found := util.GetStatus(createWorkspace, run.Status.TestCases)
	if !found {
		return fmt.Errorf("cannot find '%s' test case status", createWorkspace)
	}

	wsName, wsNamespace := ownStatus.Exports[keyWorkspaceName], ownStatus.Exports[keyWorkspaceNamespace]

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

	// Wait for deletion to complete
	err := c.waitForDeletion(ctx, wsName, wsNamespace)
	if err != nil {
		return fmt.Errorf("polling after workspace deletion failed: %w", err)
	}
	log.Info("Workspace deleted", keyWorkspaceName, ws.Name, keyWorkspaceNamespace, ws.Namespace)
	return nil
}

func (c *CreateWorkspaceTest) waitForReadyAndGet(ctx context.Context, name, namespace string) (*pwv1alpha1.Workspace, error) {
	ws := &pwv1alpha1.Workspace{}
	err := wait.PollUntilContextTimeout(ctx, 10*time.Second, 5*time.Minute, true, func(ctx context.Context) (bool, error) {
		if err := c.OnboardingClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, ws); err != nil {
			return false, err
		}
		if ws.Status.Namespace != "" {
			return true, nil
		}
		return false, nil
	})
	return ws, err
}

func (c *CreateWorkspaceTest) waitForDeletion(ctx context.Context, name, namespace string) error {
	return wait.PollUntilContextTimeout(ctx, 2*time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
		err := c.OnboardingClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, &pwv1alpha1.Workspace{})
		if errors.IsNotFound(err) {
			return true, nil // Resource is gone
		}
		if err != nil {
			return false, err // Unexpected error
		}
		return false, nil // Still exists, keep polling
	})
}
