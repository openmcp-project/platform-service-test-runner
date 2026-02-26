[![REUSE status](https://api.reuse.software/badge/github.com/openmcp-project/platform-service-test-runner)](https://api.reuse.software/info/github.com/openmcp-project/platform-service-test-runner)

# platform-service-test-runner

## About this project

Platform-service-test-runner allows to define and run in-cluster tests on the openMCP clusters. It provides a framework for creating end-to-end tests that can be executed as Kubernetes resources.

## Architecture

### E2ETestSpecification
The `E2ETestSpecification` custom resource defines a set of test cases and its configuration. It creates an `E2ETestRun` resource to execute the tests defined in the specification. 
```yaml
apiVersion: test-runner.openmcp.cloud/v1alpha1
kind: E2ETestSpecification
metadata:
  name: test-spec-
spec:
  # optional cron schedule for periodic test execution. 
  # Defaults to @daily if not specified.
  schedule: "0 0 * * *" 
  testCases:
    - name: createProject
      config: 
        # test case specific configuration can be defined here as key-value pairs
        chargingTargetType: "<some_type>"
        chargingTarget: "<some_target>"
    - name: createWorkspace
    - name: createManagedControlPlaneV2
```

### E2ETestRun
The `E2ETestRun` custom resource represents a test execution. 
It is created by the controller when an `E2ETestSpecification` is created. 
It contains the status of the test execution, including which test cases have been executed, their results, and any exports they produced for subsequent test cases.
This resource is not deleted after the test execution, but remains in the cluster for inspection and debugging purposes.
```yaml
apiVersion: test-runner.openmcp.cloud/v1alpha1
kind: E2ETestRun
metadata:
   name: test-run-dmhf6
spec:
   runner:
      version: v0.0.1
   testcases:
      - name: createProject
        config:
            # test case specific configuration will  be copied from the E2ETestSpecification here.
            chargingTargetType: "<some_type>"
            chargingTarget: "<some_target>"
      - name: createWorkspace
      - name: createManagedControlPlaneV2
status:
   testCases:
      - name: createProject
        # exports are key-value pairs that are returned by the test case and can be used by subsequent test cases. 
        # They are stored in the status of the E2ETestRun and can be accessed by test cases via the E2ETestRun.
        exports:
           project.name: test-run-dmhf6-p
           project.status.namespace: project-test-run-dmhf6-p
        # Passed or Failed depending on the result of the test case execution. 
        status: Passed
      - name: createWorkspace
        exports:
           workspace.name: test-run-dmhf6-ws
           workspace.namespace: project-test-run-dmhf6-p
           workspace.status.namespace: project-test-run-dmhf6-p--ws-test-run-dmhf6-ws
        status: Passed
      - name: createManagedControlPlaneV2
        status: Failed
        error: "Failed to create MCP"
        debugInfo: "Some debug info"
```

## Implementing New Test Cases

### TestCase Interface

All test cases must implement the `TestCase` interface defined in `internal/runner/testcase.go`:

```go
type TestCase interface {
  Run(ctx context.Context, testRun *v1alpha1.E2ETestRun, config map[string]string) (map[string]string, map[string]string, error)
  Cleanup(ctx context.Context, testRun *v1alpha1.E2ETestRun, config map[string]string) error
}
```

- **Run**: Executes the test case. Returns exports (key-value pairs for subsequent tests), error details, and error.
- **Cleanup**: Performs cleanup after the test (e.g., delete created resources).

### Example Implementation

```go
type MyTestCase struct {
    // Add any dependencies or clients needed for the test case here, e.g. Kubernetes client, specific service clients, etc.
    Client client.Client
}

func (t *MyTestCase) Run(ctx context.Context, testRun *v1alpha1.E2ETestRun, config map[string]string) (map[string]string, map[string]string, error) {
    // Use testRun to access status and exports from previous test cases.
	status := util.GetStatus(testRun, "previousTestCaseName")
	exports := status.Exports
    // Use config to access test case specific configuration.
    // Implement the test logic here, e.g. create resources, wait for conditions, etc.
    // Create own exports to be used by subsequent test cases.
    return map[string]string{}, map[string]string{}, nil
}

func (t *MyTestCase) Cleanup(ctx context.Context, testRun *v1alpha1.E2ETestRun, config map[string]string) error {
    // Clean up any resources created during Run()
    return nil
}
```

## Registering Test Cases

Test cases must be registered with the `TestRegistry` to be discoverable by the controller. Use `NewTestRegistry()` to create a registry instance.
Currently, test cases are registered in `cmd/plaform-service-test-runner/app/run.go`:
```go
testRegistry := runner.NewTestRegistry()
// register test cases here
testRegistry.RegisterTestCase("createProject", &runner.CreateProjectTest{OnboardingClient: onboardingCluster.Client()})
testRegistry.RegisterTestCase("createWorkspace", &runner.CreateWorkspaceTest{OnboardingClient: onboardingCluster.Client()})
testRegistry.RegisterTestCase("createManagedControlPlaneV2", &runner.CreateMcpTest{OnboardingClient: onboardingCluster.Client()})
```

## Requirements and Setup

In combination with the openMCP Operator, this controller can be deployed via a simple k8s resource:

```yaml
apiVersion: openmcp.cloud/v1alpha1
kind: PlatformService
metadata:
  name: test-runner
spec:
  image: ghcr.io/openmcp-project/images/platform-service-test-runner:v0.0.1
```

To run it locally, run
```shell
go run ./cmd/platform-service-test-runner/main.go init --environment default --provider-name test-runner --kubeconfig path/to/kubeconfig
```
to deploy the CRDs that are required for the controller and then
```shell
go run ./cmd/platform-service-test-runner/main.go run --environment default --provider-name test-runner --kubeconfig path/to/kubeconfig
```

## Support, Feedback, Contributing

This project is open to feature requests/suggestions, bug reports etc. via [GitHub issues](https://github.com/openmcp-project/platform-service-test-runner/issues). Contribution and feedback are encouraged and always welcome. For more information about how to contribute, the project structure, as well as additional contribution information, see our [Contribution Guidelines](CONTRIBUTING.md).

## Security / Disclosure
If you find any bug that may be a security problem, please follow our instructions at [in our security policy](https://github.com/openmcp-project/platform-service-test-runner/security/policy) on how to report it. Please do not create GitHub issues for security-related doubts or problems.

## Code of Conduct

We as members, contributors, and leaders pledge to make participation in our community a harassment-free experience for everyone. By participating in this project, you agree to abide by its [Code of Conduct](https://github.com/SAP/.github/blob/main/CODE_OF_CONDUCT.md) at all times.

## Licensing

Copyright 2026 SAP SE or an SAP affiliate company and platform-service-test-runner contributors. Please see our [LICENSE](LICENSE) for copyright and license information. Detailed information including third-party components and their licensing/copyright information is available [via the REUSE tool](https://api.reuse.software/info/github.com/openmcp-project/platform-service-test-runner).
