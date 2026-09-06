// Copyright 2026 Julia Bruner.
// Licensed under the MIT License.

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KonveerPhase defines observed delivery workflow phases.
// +kubebuilder:validation:Enum=Pending;Validating;Applying;RollingOut;Verifying;Succeeded;Failed;TimedOut
type KonveerPhase string

const (
	KonveerPhasePending KonveerPhase = "Pending"
	KonveerPhaseValidating KonveerPhase = "Validating"
	KonveerPhaseApplying KonveerPhase = "Applying"
	KonveerPhaseRollingOut KonveerPhase = "RollingOut"
	KonveerPhaseVerifying KonveerPhase = "Verifying"
	KonveerPhaseSucceeded KonveerPhase = "Succeeded"
	KonveerPhaseFailed KonveerPhase = "Failed"
	KonveerPhaseTimedOut KonveerPhase = "TimedOut"
)

// CheckResult defines the result of one Konveer workflow check.
// +kubebuilder:validation:Enum=Unknown;Passed;Failed;Skipped
type CheckResult string

const (
	CheckResultUnknown CheckResult = "Unknown"
	CheckResultPassed CheckResult = "Passed"
	CheckResultFailed CheckResult = "Failed"
	CheckResultSkipped CheckResult = "Skipped"
)

// KonveerSpec defines a lightweight delivery workflow for an existing image.
type KonveerSpec struct {
	AppRef KonveerAppReference `json:"appRef"`
	// Image is an OCI-style image reference. Konveer does not build images in the MVP.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	Image string `json:"image"`
	Mode BrunerMode `json:"mode"`
	Checks KonveerChecksSpec `json:"checks"`
	// +kubebuilder:validation:Minimum=30
	// +kubebuilder:validation:Maximum=3600
	// +kubebuilder:default:=180
	TimeoutSeconds int32 `json:"timeoutSeconds"`
}

// KonveerAppReference identifies the namespaced BrunerApp managed by the workflow.
type KonveerAppReference struct {
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	Name string `json:"name"`
}

// KonveerChecksSpec selects workflow validations and waiting behavior.
type KonveerChecksSpec struct {
	// +kubebuilder:default:=true
	ValidateImageReference bool `json:"validateImageReference"`
	// +kubebuilder:default:=true
	EnforceLatestTagPolicy bool `json:"enforceLatestTagPolicy"`
	// +kubebuilder:default:=true
	EnforceReadinessPolicy bool `json:"enforceReadinessPolicy"`
	// +kubebuilder:default:=true
	EnforceResourcePolicy bool `json:"enforceResourcePolicy"`
	// +kubebuilder:default:=true
	WaitForRollout bool `json:"waitForRollout"`
	// +kubebuilder:default:=true
	WaitForReadiness bool `json:"waitForReadiness"`
	// +kubebuilder:default:=true
	VerifyLocalEndpoint bool `json:"verifyLocalEndpoint"`
}

// KonveerStatus defines the observed delivery workflow state.
type KonveerStatus struct {
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	Phase KonveerPhase `json:"phase,omitempty"`
	AppRef KonveerObservedAppReference `json:"appRef,omitempty"`
	Rollout KonveerRolloutStatus `json:"rollout,omitempty"`
	Endpoint KonveerEndpointStatus `json:"endpoint,omitempty"`
	Checks KonveerChecksStatus `json:"checks,omitempty"`
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// KonveerObservedAppReference identifies the app observed by the workflow.
type KonveerObservedAppReference struct {
	Name string `json:"name,omitempty"`
	UID string `json:"uid,omitempty"`
}

// KonveerRolloutStatus records rollout timing and generation.
type KonveerRolloutStatus struct {
	StartedAt *metav1.Time `json:"startedAt,omitempty"`
	CompletedAt *metav1.Time `json:"completedAt,omitempty"`
	DeploymentGeneration int64 `json:"deploymentGeneration,omitempty"`
}

// KonveerEndpointStatus records local endpoint verification.
type KonveerEndpointStatus struct {
	URL string `json:"url,omitempty"`
	Verified bool `json:"verified,omitempty"`
	StatusCode int32 `json:"statusCode,omitempty"`
}

// KonveerChecksStatus records all selected workflow check results.
type KonveerChecksStatus struct {
	ImageReference CheckResult `json:"imageReference,omitempty"`
	Policy CheckResult `json:"policy,omitempty"`
	Rollout CheckResult `json:"rollout,omitempty"`
	Readiness CheckResult `json:"readiness,omitempty"`
	LocalhostEndpoint CheckResult `json:"localhostEndpoint,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,shortName=knv
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="App",type=string,JSONPath=`.spec.appRef.name`
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=`.spec.image`
// +kubebuilder:printcolumn:name="Endpoint Verified",type=boolean,JSONPath=`.status.endpoint.verified`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Konveer represents a lightweight delivery workflow for an existing image.
type Konveer struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec KonveerSpec `json:"spec,omitempty"`
	Status KonveerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// KonveerList contains a list of Konveer resources.
type KonveerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items []Konveer `json:"items"`
}
