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
	createProject             = "createProject"
	keyProjectName            = "project.name"
	keyProjectStatusNamespace = "project.status.namespace"
	configChargingTargetType  = "chargingTargetType"
	configChargingTarget      = "chargingTarget"
	labelChargingTargetType   = "openmcp.cloud.sap/charging-target-type"
	labelChargingTarget       = "openmcp.cloud.sap/charging-target"
)

type CreateProjectTest struct {
	OnboardingClient client.Client
}

// Run creates a project with the given configuration and waits until it's ready.
// It returns the project name and status namespace as exports for other test cases to use.
// It reads chargingTarget and chargingTargetType from the config to set labels for cost allocation, if provided.
func (c *CreateProjectTest) Run(ctx context.Context, run *v1alpha1.E2ETestRun, config map[string]string) (map[string]string, map[string]string, error) {
	log := logging.FromContextOrPanic(ctx).WithName(createProject)
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)

	projectName := fmt.Sprintf("%s-p", run.Name)
	members := []pwv1alpha1.ProjectMember{
		{
			Subject: pwv1alpha1.Subject{
				Kind: v1.UserKind,
				Name: config[configIdentity],
			},
			Roles: []pwv1alpha1.ProjectMemberRole{
				pwv1alpha1.ProjectRoleAdmin,
			},
		},
	}

	labels := map[string]string{labelTestCase: createProject}
	if config[configChargingTarget] != "" && config[configChargingTargetType] != "" {
		labels[labelChargingTarget] = config[configChargingTarget]
		labels[labelChargingTargetType] = config[configChargingTargetType]
	}

	project := &pwv1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name:   projectName,
			Labels: labels,
		},
		Spec: pwv1alpha1.ProjectSpec{Members: members},
	}

	defer cancel()
	if err := c.OnboardingClient.Create(ctx, project); err != nil {
		return nil, nil, fmt.Errorf("project creation failed: %w", err)
	}
	log.Debug("Project created", "name", project.Name)
	project, err := c.waitForReadyAndGet(ctx, projectName, "")
	if err != nil {
		return nil, nil, fmt.Errorf("polling after project creation failed: %w", err)
	}
	log.Debug("Project ready", keyProjectName, project.Name, keyProjectStatusNamespace, project.Status.Namespace)
	return map[string]string{
		keyProjectName:            project.Name,
		keyProjectStatusNamespace: project.Status.Namespace,
	}, nil, nil
}

// Cleanup deletes the project created in the Run method, identified via own export. It waits until the project is fully deleted before returning.
func (c *CreateProjectTest) Cleanup(ctx context.Context, run *v1alpha1.E2ETestRun, _ map[string]string) error {
	log := logging.FromContextOrPanic(ctx).WithName(createProject)
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	ownStatus, found := util.GetStatus(createProject, run.Status.TestCases)
	if !found {
		return fmt.Errorf("cannot find '%s' test case status", createProject)
	}
	projectName := ownStatus.Exports[keyProjectName]

	project := &pwv1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name: projectName,
		},
	}
	log.Debug("Deleting project", keyProjectName, projectName)
	if err := c.OnboardingClient.Delete(ctx, project); err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("project deletion failed: %w", err)
		}
		return nil // Already gone
	}

	// Wait for deletion to complete
	err := c.waitForDeletion(ctx, projectName)
	if err != nil {
		return fmt.Errorf("polling after project deletion failed: %w", err)
	}
	log.Debug("Project deleted", keyProjectName, project.Name)
	return nil
}

func (c *CreateProjectTest) waitForReadyAndGet(ctx context.Context, name, namespace string) (*pwv1alpha1.Project, error) {
	project := &pwv1alpha1.Project{}
	err := wait.PollUntilContextTimeout(ctx, 10*time.Second, 5*time.Minute, true, func(ctx context.Context) (bool, error) {
		if err := c.OnboardingClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, project); err != nil {
			return false, err
		}
		if project.Status.Namespace != "" {
			return true, nil
		}
		return false, nil
	})
	return project, err
}

func (c *CreateProjectTest) waitForDeletion(ctx context.Context, name string) error {
	return wait.PollUntilContextTimeout(ctx, 2*time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
		err := c.OnboardingClient.Get(ctx, types.NamespacedName{Name: name}, &pwv1alpha1.Project{})
		if errors.IsNotFound(err) {
			return true, nil // Resource is gone
		}
		if err != nil {
			return false, err // Unexpected error
		}
		return false, nil // Still exists, keep polling
	})
}
