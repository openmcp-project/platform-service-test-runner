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
	"sigs.k8s.io/controller-runtime/pkg/client"

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
	defer cancel()

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

	if err := c.OnboardingClient.Create(ctx, project); err != nil {
		return nil, nil, fmt.Errorf("project creation failed: %w", err)
	}

	log.Debug("Project created", "name", project.Name)

	project, err := WaitForReadyAndGet(ctx, c.OnboardingClient, projectName, "", project, defaultPollInterval, defaultPollTimeout, IsProjectReady)
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

	ownStatus, found := GetStatus(createProject, run.Status.TestCases)
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
	err := WaitForDeletion(ctx, c.OnboardingClient, projectName, "", &pwv1alpha1.Project{}, defaultPollInterval, defaultPollTimeout)
	if err != nil {
		return fmt.Errorf("polling after project deletion failed: %w", err)
	}

	log.Debug("Project deleted", keyProjectName, project.Name)

	return nil
}

// IsProjectReady checks if a Project is ready by verifying its status namespace is set.
func IsProjectReady(project *pwv1alpha1.Project) bool {
	return project.Status.Namespace != ""
}
