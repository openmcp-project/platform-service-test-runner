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
  name: test-spec
spec:
  # optional cron schedule for periodic test execution. 
  # Defaults to @daily if not specified.
  schedule: "0 0 * * *" 
  testCases:
    - name: createProject
      config: {}
    - name: createWorkspace
      config: {}
    - name: createManagedControlPlaneV2
      config: {}
```

#### Test case configuration
Each test case in the `E2ETestSpecification` can have its own configuration, which is passed to the test case implementation when it is executed.

Currently, the following configuration keys exist:

| Key | Used by | Description |
| --- | --- | --- |
| `chargingTarget` | `createProject` | Specifies the charging target for the project. Value should be a UUID. |
| `chargingTargetType` | `createProject` | Specifies the type of the charging target for the project. |
| `identity` | All test cases | Automatically added by the controller, contains the username retrieved via `authenticationv1.SelfSubjectReview{}`. |
| `pollInterval` | All test cases | Polling interval for waiting for conditions. Duration string (e.g., "30s"). Defaults to 10 seconds if not specified. |
| `pollTimeout` | All test cases | Polling timeout for waiting for conditions. Duration string (e.g., "5m"). Defaults to 5 minutes if not specified. |



### E2ETestRun
The `E2ETestRun` custom resource represents a test execution. 
It is created by the `e2etestspecification_controller.go` controller when an `E2ETestSpecification` is created and a new test run needs to be started (based on the schedule).
It contains the status of the test execution, including which test cases have been executed, their results, and any exports they produced for subsequent test cases.
This resource is not deleted after the test execution, but remains in the cluster for inspection and debugging purposes. 
Example:
```yaml
apiVersion: test-runner.openmcp.cloud/v1alpha1
kind: E2ETestRun
metadata:
  labels:
    test-runner.openmcp.cloud/specification: test-create-p-w-m
  name: run-1772628360009
spec:
  runner:
    version: 0.0.1-dev
  testcases:
    - config:
        chargingTarget: 12345678-1234-1234-1234-123456789012
        chargingTargetType: btp
      name: createProject
    - name: createWorkspace
    - config:
        pollInterval: 20s
        pollTimeout: 15m
      name: createManagedControlPlaneV2
      
status:
  testCases:
    - conditions:
        - lastTransitionTime: "2026-03-04T13:01:04Z"
          message: Test case cleanup completed successfully
          reason: CleanupSuccess
          status: "True"
          type: CleanupCompleted
        - lastTransitionTime: "2026-03-04T12:59:54Z"
          message: Test case run completed successfully
          reason: TestPassed
          status: "True"
          type: RunCompleted
      exports:
        project.name: run-1772628360009-p
        project.status.namespace: project-run-1772628360009-p
      name: createProject
    - conditions:
        - lastTransitionTime: "2026-03-04T13:00:54Z"
          message: Test case cleanup completed successfully
          reason: CleanupSuccess
          status: "True"
          type: CleanupCompleted
        - lastTransitionTime: "2026-03-04T13:00:04Z"
          message: Test case run completed successfully
          reason: TestPassed
          status: "True"
          type: RunCompleted
      exports:
        workspace.name: run-1772628360009-ws
        workspace.namespace: project-run-1772628360009-p
        workspace.status.namespace: project-run-1772628360009-p--ws-run-1772628360009-ws
      name: createWorkspace
    - conditions:
        - lastTransitionTime: "2026-03-04T13:00:44Z"
          message: Test case cleanup completed successfully
          reason: CleanupSuccess
          status: "True"
          type: CleanupCompleted
        - lastTransitionTime: "2026-03-04T13:00:14Z"
          message: Test case run completed successfully
          reason: TestPassed
          status: "True"
          type: RunCompleted
      exports:
        mcp.name: run-1772628360009-mcpv2
        mcp.namespace: project-run-1772628360009-p--ws-run-1772628360009-ws
      name: createManagedControlPlaneV2
```

## Implementing New Test Cases

### TestCase Interface

All test cases must implement the `TestCase` interface defined in `internal/runner/testcase.go`:

```go
type TestCase interface {
  Run(ctx context.Context, testRun *v1alpha1.E2ETestRun, config Config) (Exports, DebugInfo, error)
  Cleanup(ctx context.Context, testRun *v1alpha1.E2ETestRun, config Config) error
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

func (t *MyTestCase) Run(ctx context.Context, testRun *v1alpha1.E2ETestRun, config Config (Exports, DebugInfo, error) {
    // Use testRun to access status and exports from previous test cases. 
    status := util.GetStatus(testRun, "previousTestCaseName")
    exports := status.Exports
    // Use config to access test case specific configuration.
    // By default, the controller is adding a key `identity` to the config, which is its own username retrieved via authenticationv1.SelfSubjectReview{}
    // Implement the test logic here, e.g. create resources, wait for conditions, etc.
    // Create own Exports to be used by subsequent test cases.
    return Exports{}, DebugInfo{}, nil
}

func (t *MyTestCase) Cleanup(ctx context.Context, testRun *v1alpha1.E2ETestRun, config Config) error {
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
