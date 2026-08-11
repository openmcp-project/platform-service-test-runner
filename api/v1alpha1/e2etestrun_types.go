package v1alpha1

import (
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Runner defines the test runner image version and arguments.
type Runner struct {
	Version string   `json:"version"`
	Args    []string `json:"args,omitempty"`
}

// E2ETestRunSpec defines the desired state of E2ETestRun
type E2ETestRunSpec struct {
	Runner    Runner     `json:"runner"`
	TestCases []TestCase `json:"testcases"`
}

// Condition types for TestCaseStatus
const (
	// TestCaseConditionRunCompleted indicates whether the test case run phase completed
	TestCaseConditionRunCompleted = "RunCompleted"
	// TestCaseConditionCleanupCompleted indicates whether the test case cleanup phase completed
	TestCaseConditionCleanupCompleted = "CleanupCompleted"
)

// Condition reasons for TestCaseStatus
const (
	// TestCaseReasonPassed indicates the test case passed all checks
	TestCaseReasonPassed = "TestPassed"
	// TestCaseReasonFailed indicates the test case failed
	TestCaseReasonFailed = "TestFailed"
	// TestCaseReasonCleanupError indicates an error occurred during test cleanup
	TestCaseReasonCleanupError = "CleanupError"
	// TestCaseReasonCleanupSuccess indicates the cleanup completed successfully
	TestCaseReasonCleanupSuccess = "CleanupSuccess"
)

// TestCaseStatus holds the status of a single test case execution.
type TestCaseStatus struct {
	Name string `json:"name"`
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	Exports json.RawMessage `json:"exports,omitempty"`
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	DebugInfo json.RawMessage `json:"debugInfo,omitempty"`
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// E2ETestRunStatus defines the observed state of E2ETestRun.
type E2ETestRunStatus struct {
	TestCases []TestCaseStatus `json:"testCases,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:metadata:labels="openmcp.cloud/cluster=platform"
// +kubebuilder:resource:scope=Cluster

// E2ETestRun is the Schema for the e2etestruns API
type E2ETestRun struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of E2ETestRun
	// +required
	Spec E2ETestRunSpec `json:"spec"`

	// status defines the observed state of E2ETestRun
	// +optional
	Status E2ETestRunStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// E2ETestRunList contains a list of E2ETestRun
type E2ETestRunList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []E2ETestRun `json:"items"`
}

func init() {
	RegisterToSchemeBuilder(&E2ETestRun{}, &E2ETestRunList{})
}
