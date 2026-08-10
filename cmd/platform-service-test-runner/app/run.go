package app

import (
	"context"
	"crypto/tls"
	"fmt"
	"os"
	"path/filepath"
	"time"

	clustersv1alpha1 "github.com/openmcp-project/openmcp-operator/api/clusters/v1alpha1"
	"github.com/spf13/cobra"
	rbacv1 "k8s.io/api/rbac/v1"
	"sigs.k8s.io/controller-runtime/pkg/healthz"

	"github.com/openmcp-project/platform-service-test-runner/internal/controller/e2etestrun"
	"github.com/openmcp-project/platform-service-test-runner/internal/runner"
	"github.com/openmcp-project/platform-service-test-runner/internal/version"

	"github.com/openmcp-project/platform-service-test-runner/internal/controller/e2etestspec"

	"github.com/openmcp-project/controller-utils/pkg/logging"
	openmcpconst "github.com/openmcp-project/openmcp-operator/api/constants"
	"github.com/openmcp-project/openmcp-operator/lib/clusteraccess"
	authenticationv1 "k8s.io/api/authentication/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/certwatcher"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	providerscheme "github.com/openmcp-project/platform-service-test-runner/api/install"
)

var setupLog logging.Logger

func NewRunCommand(so *SharedOptions) *cobra.Command {
	opts := &RunOptions{
		SharedOptions: so,
	}
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the Platform Service Test Runner",
		Run: func(cmd *cobra.Command, args []string) {
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
	opts.AddFlags(cmd)

	return cmd
}

type RawRunOptions struct {
	// kubebuilder default flags
	MetricsAddr          string `json:"metrics-bind-address"`
	MetricsCertPath      string `json:"metrics-cert-path"`
	MetricsCertName      string `json:"metrics-cert-name"`
	MetricsCertKey       string `json:"metrics-cert-key"`
	EnableLeaderElection bool   `json:"leader-elect"`
	ProbeAddr            string `json:"health-probe-bind-address"`
	PprofAddr            string `json:"pprof-bind-address"`
	SecureMetrics        bool   `json:"metrics-secure"`
	EnableHTTP2          bool   `json:"enable-http2"`
}

type RunOptions struct {
	*SharedOptions
	RawRunOptions

	// fields filled in Complete()
	TLSOpts              []func(*tls.Config)
	MetricsServerOptions metricsserver.Options
	MetricsCertWatcher   *certwatcher.CertWatcher
	ProviderNamespace    string
}

func (o *RunOptions) AddFlags(cmd *cobra.Command) {
	// kubebuilder default flags
	cmd.Flags().StringVar(&o.MetricsAddr, "metrics-bind-address", "0", "The address the metrics endpoint binds to. Use :8443 for HTTPS or :8080 for HTTP, or leave as 0 to disable the metrics service.")
	cmd.Flags().StringVar(&o.ProbeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	cmd.Flags().StringVar(&o.PprofAddr, "pprof-bind-address", "", "The address the pprof endpoint binds to. Expected format is ':<port>'. Leave empty to disable pprof endpoint.")
	cmd.Flags().BoolVar(&o.EnableLeaderElection, "leader-elect", false, "Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	cmd.Flags().BoolVar(&o.SecureMetrics, "metrics-secure", true, "If set, the metrics endpoint is served securely via HTTPS. Use --metrics-secure=false to use HTTP instead.")
	cmd.Flags().StringVar(&o.MetricsCertPath, "metrics-cert-path", "", "The directory that contains the metrics server certificate.")
	cmd.Flags().StringVar(&o.MetricsCertName, "metrics-cert-name", "tls.crt", "The name of the metrics server certificate file.")
	cmd.Flags().StringVar(&o.MetricsCertKey, "metrics-cert-key", "tls.key", "The name of the metrics server key file.")
	cmd.Flags().BoolVar(&o.EnableHTTP2, "enable-http2", false, "If set, HTTP/2 will be enabled for the metrics and webhook servers")
}

func (o *RunOptions) Complete(ctx context.Context) error {
	if err := o.SharedOptions.Complete(); err != nil {
		return err
	}
	o.ProviderNamespace = os.Getenv(openmcpconst.EnvVariablePodNamespace)
	if o.ProviderNamespace == "" {
		return fmt.Errorf("environment variable '%s' must be set", openmcpconst.EnvVariablePodNamespace)
	}

	setupLog = o.Log.WithName("setup")
	ctrl.SetLogger(o.Log.Logr())

	// kubebuilder default stuff

	// if the enable-http2 flag is false (the default), http/2 should be disabled
	// due to its vulnerabilities. More specifically, disabling http/2 will
	// prevent from being vulnerable to the HTTP/2 Stream Cancellation and
	// Rapid Reset CVEs. For more information see:
	// - https://github.com/advisories/GHSA-qppj-fm5r-hxr3
	// - https://github.com/advisories/GHSA-4374-p667-p6c8
	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("Disabling http/2")
		c.NextProtos = []string{"http/1.1"}
	}

	if !o.EnableHTTP2 {
		o.TLSOpts = append(o.TLSOpts, disableHTTP2)
	}

	// More info:
	// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.2/pkg/metrics/server
	// - https://book.kubebuilder.io/reference/metrics.html
	o.MetricsServerOptions = metricsserver.Options{
		BindAddress:   o.MetricsAddr,
		SecureServing: o.SecureMetrics,
		TLSOpts:       o.TLSOpts,
	}

	if o.SecureMetrics {
		// FilterProvider is used to protect the metrics endpoint with authn/authz.
		// These configurations ensure that only authorized users and service accounts
		// can access the metrics endpoint. The RBAC are configured in 'config/rbac/kustomization.yaml'. More info:
		// https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.2/pkg/metrics/filters#WithAuthenticationAndAuthorization
		o.MetricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization
	}

	// If the certificate is not specified, controller-runtime will automatically
	// generate self-signed certificates for the metrics server. While convenient for development and testing,
	// this setup is not recommended for production.
	//
	// TODO(user): If you enable certManager, uncomment the following lines:
	// - [METRICS-WITH-CERTS] at config/default/kustomization.yaml to generate and use certificates
	// managed by cert-manager for the metrics server.
	// - [PROMETHEUS-WITH-CERTS] at config/prometheus/kustomization.yaml for TLS certification.
	if len(o.MetricsCertPath) > 0 {
		setupLog.Info("Initializing metrics certificate watcher using provided certificates", "metrics-cert-path", o.MetricsCertPath, "metrics-cert-name", o.MetricsCertName, "metrics-cert-key", o.MetricsCertKey)

		var err error
		o.MetricsCertWatcher, err = certwatcher.New(
			filepath.Join(o.MetricsCertPath, o.MetricsCertName),
			filepath.Join(o.MetricsCertPath, o.MetricsCertKey),
		)
		if err != nil {
			return fmt.Errorf("failed to initialize metrics certificate watcher: %w", err)
		}

		o.MetricsServerOptions.TLSOpts = append(o.MetricsServerOptions.TLSOpts, func(config *tls.Config) {
			config.GetCertificate = o.MetricsCertWatcher.GetCertificate
		})
	}

	return nil
}

func (o *RunOptions) Run(ctx context.Context) error {
	if err := o.PlatformCluster.InitializeClient(providerscheme.InstallOperatorAPIsPlatform(runtime.NewScheme())); err != nil {
		return err
	}

	setupLog = o.Log.WithName("setup")
	setupLog.Info("Environment", "value", o.Environment)
	setupLog.Info("ProviderName", "value", o.ProviderName)

	// Get version from build-time injected variable or fallback to VERSION file
	appVersion := version.GetVersion()
	setupLog.Info("Version", "value", appVersion)

	clusterAccessManager := clusteraccess.NewClusterAccessManager(o.PlatformCluster.Client(), "test-runner.openmcp.cloud", os.Getenv("POD_NAMESPACE"))
	clusterAccessManager.WithLogger(&setupLog).
		WithInterval(10 * time.Second).
		WithTimeout(30 * time.Minute)

	onboardingCluster, err := clusterAccessManager.CreateAndWaitForCluster(ctx, "onboarding-run", clustersv1alpha1.PURPOSE_ONBOARDING,
		providerscheme.InstallOperatorAPIsOnboarding(runtime.NewScheme()), []clustersv1alpha1.PermissionsRequest{
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
					// TODO: temporarily grant full access for *.services.open-control-plane.io
					{
						APIGroups: []string{"*"},
						Resources: []string{"*"},
						Verbs:     []string{"*"},
					},
				},
			},
		})

	if err != nil {
		setupLog.Error(err, "Failed to create and wait for onboarding cluster")
		return fmt.Errorf("failed to create and wait for onboarding cluster: %w", err)
	}

	// figure out own identity
	review := &authenticationv1.SelfSubjectReview{}
	if err := onboardingCluster.Client().Create(ctx, review); err != nil {
		return fmt.Errorf("failed to get own identity: %w", err)
	}
	identity := review.Status.UserInfo.Username

	mgr, err := ctrl.NewManager(o.PlatformCluster.RESTConfig(), ctrl.Options{
		Scheme:                 o.PlatformCluster.Scheme(),
		Metrics:                o.MetricsServerOptions,
		HealthProbeBindAddress: o.ProbeAddr,
		PprofBindAddress:       o.PprofAddr,
		LeaderElection:         o.EnableLeaderElection,
		LeaderElectionID:       "github.com/openmcp-project/platform-service-test-runner",
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		//
		// In the default scaffold provided, the program ends immediately after
		// the manager stops, so would be fine to enable this option. However,
		// if you are doing or is intended to do any operation such as perform cleanups
		// after the manager stops then its usage might be unsafe.
		// LeaderElectionReleaseOnCancel: true,
	})
	if err != nil {
		return fmt.Errorf("unable to create manager: %w", err)
	}

	// setup TestSpec reconciler
	if err := e2etestspec.NewE2ETestSpecificationReconciler(o.PlatformCluster, appVersion).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to add E2ETestSpecificationReconciler to manager: %w", err)
	}

	testRegistry := runner.NewTestRegistry()
	// register test cases here
	testRegistry.RegisterTestCase("createProject", &runner.CreateProjectTest{OnboardingClient: onboardingCluster.Client()})
	testRegistry.RegisterTestCase("createWorkspace", &runner.CreateWorkspaceTest{OnboardingClient: onboardingCluster.Client()})
	testRegistry.RegisterTestCase("createControlPlane", &runner.CreateControlPlaneTest{OnboardingClient: onboardingCluster.Client()})
	testRegistry.RegisterTestCase("createService", &runner.CreateServiceTest{OnboardingClient: onboardingCluster.Client()})

	// setup TestRun reconciler
	if err := e2etestrun.NewE2ETestRunReconciler(o.PlatformCluster, mgr.GetEventRecorder(e2etestrun.ControllerName), identity, testRegistry).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to add E2ETestRunReconciler to manager: %w", err)
	}

	if o.MetricsCertWatcher != nil {
		setupLog.Info("Adding metrics certificate watcher to manager")
		if err := mgr.Add(o.MetricsCertWatcher); err != nil {
			return fmt.Errorf("unable to add metrics certificate watcher to manager: %w", err)
		}
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return fmt.Errorf("unable to set up health check: %w", err)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return fmt.Errorf("unable to set up ready check: %w", err)
	}

	setupLog.Info("Starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		return fmt.Errorf("problem running manager: %w", err)
	}

	return nil
}
