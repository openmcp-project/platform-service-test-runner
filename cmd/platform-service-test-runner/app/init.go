package app

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime"

	localcrds "github.com/openmcp-project/platform-service-test-runner/api/crds"

	crdutil "github.com/openmcp-project/controller-utils/pkg/crds"
	clustersv1alpha1 "github.com/openmcp-project/openmcp-operator/api/clusters/v1alpha1"
	openmcpconst "github.com/openmcp-project/openmcp-operator/api/constants"
	openmcpcrds "github.com/openmcp-project/openmcp-operator/api/crds"
	"github.com/openmcp-project/openmcp-operator/lib/clusteraccess"
	pwcrds "github.com/openmcp-project/project-workspace-operator/api/crds"

	providerscheme "github.com/openmcp-project/platform-service-test-runner/api/install"
)

// NewInitCommand creates the cobra command for the init subcommand.
func NewInitCommand(so *SharedOptions) *cobra.Command {
	opts := &InitOptions{
		SharedOptions: so,
	}
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize Platform Service Test Runner",
		Run: func(cmd *cobra.Command, _ []string) {
			opts.PrintRawOptions(cmd)
			if err := opts.Complete(cmd.Context()); err != nil {
				panic(fmt.Errorf("error completing options: %w", err))
			}
			opts.PrintCompletedOptions(cmd)
			if opts.DryRun {
				cmd.Println("=== END OF DRY RUN ===")
				return
			}
			if err := opts.Run(cmd.Context()); err != nil {
				panic(err)
			}
		},
	}

	return cmd
}

// InitOptions holds options for the init subcommand.
type InitOptions struct {
	*SharedOptions
}

// Complete validates and resolves the init options.
func (o *InitOptions) Complete(_ context.Context) error {
	return o.SharedOptions.Complete()
}

// Run executes the init logic: creates cluster access, applies CRDs, and sets up required resources.
func (o *InitOptions) Run(ctx context.Context) error {
	if err := o.PlatformCluster.InitializeClient(providerscheme.OperatorAPIsPlatform(runtime.NewScheme())); err != nil {
		return err
	}

	log := o.Log.WithName("main")
	log.Info("Environment", "value", o.Environment)
	log.Info("ProviderName", "value", o.ProviderName)

	// Create cluster access manager for onboarding cluster
	clusterAccessManager := clusteraccess.NewClusterAccessManager(o.PlatformCluster.Client(), "test-runner.openmcp.cloud", os.Getenv("POD_NAMESPACE"))
	clusterAccessManager.WithLogger(&log).
		WithInterval(10 * time.Second).
		WithTimeout(30 * time.Minute)

	onboardingCluster, err := clusterAccessManager.CreateAndWaitForCluster(ctx, "onboarding-init", clustersv1alpha1.PURPOSE_ONBOARDING,
		providerscheme.OperatorAPIsOnboarding(runtime.NewScheme()), []clustersv1alpha1.PermissionsRequest{
			{
				Rules: []rbacv1.PolicyRule{
					// openmcp-operator CRDs (ControlPlane, etc.)
					{
						APIGroups: []string{"clusters.openmcp.cloud", "core.open-control-plane.io"},
						Resources: []string{"*"},
						Verbs:     []string{"*"},
					},
					// project-workspace-operator CRDs
					{
						APIGroups: []string{"core.openmcp.cloud"},
						Resources: []string{"projects", "workspaces"},
						Verbs:     []string{"*"},
					},
					// CRD management (apiextensions)
					{
						APIGroups: []string{"apiextensions.k8s.io"},
						Resources: []string{"customresourcedefinitions"},
						Verbs:     []string{"*"},
					},
					// Core resources (namespaces, secrets, configmaps for cluster access)
					{
						APIGroups: []string{""},
						Resources: []string{"namespaces", "secrets", "configmaps"},
						Verbs:     []string{"*"},
					},
				},
			},
		})
	if err != nil {
		return fmt.Errorf("error creating onboarding cluster access: %w", err)
	}

	// apply openmcp-operator CRDs (includes ControlPlane, etc.)
	crdManager := crdutil.NewCRDManager(openmcpconst.ClusterLabel, openmcpcrds.CRDs)
	crdManager.AddCRDLabelToClusterMapping(clustersv1alpha1.PURPOSE_PLATFORM, o.PlatformCluster)
	crdManager.AddCRDLabelToClusterMapping(clustersv1alpha1.PURPOSE_ONBOARDING, onboardingCluster)
	if err := crdManager.CreateOrUpdateCRDs(ctx, &log); err != nil {
		return fmt.Errorf("error creating/updating openmcp CRDs: %w", err)
	}

	// apply project-workspace-operator CRDs (Project, Workspace)
	pwCRDManager := crdutil.NewCRDManager(openmcpconst.ClusterLabel, pwcrds.CRDs)
	pwCRDManager.AddCRDLabelToClusterMapping(clustersv1alpha1.PURPOSE_PLATFORM, o.PlatformCluster)
	pwCRDManager.AddCRDLabelToClusterMapping(clustersv1alpha1.PURPOSE_ONBOARDING, onboardingCluster)
	if err := pwCRDManager.CreateOrUpdateCRDs(ctx, &log); err != nil {
		return fmt.Errorf("error creating/updating project-workspace CRDs: %w", err)
	}

	// apply local test-runner CRDs (E2ETestRun, E2ETestSpecification)
	localCRDManager := crdutil.NewCRDManager(openmcpconst.ClusterLabel, localcrds.CRDs)
	localCRDManager.AddCRDLabelToClusterMapping(clustersv1alpha1.PURPOSE_PLATFORM, o.PlatformCluster)
	if err := localCRDManager.CreateOrUpdateCRDs(ctx, &log); err != nil {
		return fmt.Errorf("error creating/updating local CRDs: %w", err)
	}

	log.Info("Finished init command")
	return nil
}
