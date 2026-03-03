package v1alpha1

import (
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type TestCase struct {
	Name string `json:"name"`
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	// +optional
	Config json.RawMessage `json:"config,omitempty"`
}

// E2ETestSpecificationSpec defines the desired state of E2ETestSpecification
type E2ETestSpecificationSpec struct {
	// +optional
	// +kubebuilder:default="@daily"
	Schedule string `json:"schedule,omitempty"`
	// TestCases is a list of test cases to be executed in order. Each test case includes a name and optional configuration.
	TestCases []TestCase `json:"testCases"`
}

type E2ETestSpecificationStatus struct {
	LastExecutionTime *metav1.Time `json:"lastExecutionTime"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:metadata:labels="openmcp.cloud/cluster=platform"
// +kubebuilder:resource:scope=Cluster

// E2ETestSpecification is the Schema for the e2etestspecifications API
type E2ETestSpecification struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of E2ETestSpecification
	// +required
	Spec E2ETestSpecificationSpec `json:"spec"`

	// status defines the observed state of E2ETestSpecification
	// +optional
	Status E2ETestSpecificationStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// E2ETestSpecificationList contains a list of E2ETestSpecification
type E2ETestSpecificationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []E2ETestSpecification `json:"items"`
}

func init() {
	SchemeBuilder.Register(&E2ETestSpecification{}, &E2ETestSpecificationList{})
}
