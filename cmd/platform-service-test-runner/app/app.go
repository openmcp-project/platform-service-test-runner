package app

import (
	"fmt"
	"os"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/spf13/cobra"

	"github.com/openmcp-project/controller-utils/pkg/clusters"
	"github.com/openmcp-project/controller-utils/pkg/logging"
)

// NewPlatformServiceTestRunnerCommand creates the root cobra command for the platform-service-test-runner.
func NewPlatformServiceTestRunnerCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "platform-service-test-runner",
		Short: "platform-service-test-runner allows to run in-cluster test cases",
	}
	cmd.SetOut(os.Stdout)
	cmd.SetErr(os.Stderr)

	so := &SharedOptions{
		RawSharedOptions: &RawSharedOptions{},
		PlatformCluster:  clusters.New("platform"),
	}
	so.AddPersistentFlags(cmd)
	cmd.AddCommand(NewInitCommand(so))
	cmd.AddCommand(NewRunCommand(so))

	return cmd
}

// RawSharedOptions holds the raw flag values for options shared across subcommands.
type RawSharedOptions struct {
	Environment  string `json:"environment"`
	ProviderName string `json:"provider-name"`
	DryRun       bool   `json:"dry-run"`
}

// SharedOptions holds options common to all subcommands, combining raw flag values with resolved runtime state.
type SharedOptions struct {
	*RawSharedOptions
	PlatformCluster *clusters.Cluster

	// fields filled in Complete()
	Log logging.Logger
}

// AddPersistentFlags registers all persistent flags for the shared options on the given command.
func (o *SharedOptions) AddPersistentFlags(cmd *cobra.Command) {
	// logging
	logging.InitFlags(cmd.PersistentFlags())
	// clusters
	o.PlatformCluster.RegisterSingleConfigPathFlag(cmd.PersistentFlags())
	// environment
	cmd.PersistentFlags().StringVar(&o.Environment, "environment", "", "Environment name. Required. This is used to distinguish between different environments that are watching the same Onboarding cluster. Must be globally unique.")
	// provider name
	cmd.PersistentFlags().StringVar(&o.ProviderName, "provider-name", "", "Name of the provider resource.")
	cmd.PersistentFlags().BoolVar(&o.DryRun, "dry-run", false, "If set, the command aborts after evaluation of the given flags.")
}

// Complete validates and resolves the shared options, building the logger and initialising the platform cluster REST config.
func (o *SharedOptions) Complete() error {
	if o.Environment == "" {
		return fmt.Errorf("environment must not be empty")
	}
	if o.ProviderName == "" {
		return fmt.Errorf("provider-name must not be empty")
	}

	// build logger
	log, err := logging.GetLogger()
	if err != nil {
		return err
	}
	o.Log = log
	ctrl.SetLogger(o.Log.Logr())

	if err := o.PlatformCluster.InitializeRESTConfig(); err != nil {
		return err
	}

	return nil
}
