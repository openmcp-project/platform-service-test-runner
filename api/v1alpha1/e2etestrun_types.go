package v1alpha1

import (
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Runner struct {
	Version string   `json:"version"`
	Args    []string `json:"args,omitempty"`
}

// E2ETestRunSpec defines the desired state of E2ETestRun
type E2ETestRunSpec struct {
	Runner    Runner     `json:"runner"`
	TestCases []TestCase `json:"testcases"`
}

type TestCaseStatus struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	Exports json.RawMessage `json:"exports,omitempty"`
	Error   string          `json:"error,omitempty"`
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	DebugInfo  json.RawMessage    `json:"debugInfo,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
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
	SchemeBuilder.Register(&E2ETestRun{}, &E2ETestRunList{})
}
