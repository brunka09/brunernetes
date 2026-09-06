// Copyright 2026 Julia Bruner.
// Licensed under the MIT License.

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LatestTagBehavior defines the policy applied to :latest image tags.
// +kubebuilder:validation:Enum=Warn;Reject
type LatestTagBehavior string

const (
	LatestTagBehaviorWarn   LatestTagBehavior = "Warn"
	LatestTagBehaviorReject LatestTagBehavior = "Reject"
)

// AvtoVAZSpec defines factory-wide configuration and policy.
type AvtoVAZSpec struct {
	Defaults AvtoVAZDefaultsSpec `json:"defaults"`
	Safety AvtoVAZSafetyPolicy `json:"safety"`
	Quality AvtoVAZQualityPolicy `json:"quality"`
	Capacity AvtoVAZCapacityPolicy `json:"capacity"`
	Friday AvtoVAZFridayPolicy `json:"friday"`
	Rollout AvtoVAZRolloutPolicy `json:"rollout"`
}

// AvtoVAZDefaultsSpec defines default Bruner mode values.
type AvtoVAZDefaultsSpec struct {
	BrunerMode BrunerMode `json:"brunerMode"`
}

// AvtoVAZSafetyPolicy defines local workload safety requirements.
type AvtoVAZSafetyPolicy struct {
	// +kubebuilder:default:=false
	RequireReadinessProbe bool `json:"requireReadinessProbe"`
	// +kubebuilder:default:=false
	RequireResourceRequests bool `json:"requireResourceRequests"`
	// +kubebuilder:default:=false
	AllowPrivilegedContainers bool `json:"allowPrivilegedContainers"`
	// +kubebuilder:default:=false
	AllowHostNetwork bool `json:"allowHostNetwork"`
	// +kubebuilder:default:=false
	AllowHostPID bool `json:"allowHostPID"`
	// +kubebuilder:default:=false
	AllowHostPathVolumes bool `json:"allowHostPathVolumes"`
}

// AvtoVAZQualityPolicy defines local image and health quality requirements.
type AvtoVAZQualityPolicy struct {
	// +kubebuilder:default:=Warn
	LatestTagBehavior LatestTagBehavior `json:"latestTagBehavior"`
	// +kubebuilder:validation:Enum=Always;IfNotPresent;Never
	// +kubebuilder:default:=IfNotPresent
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy"`
	// +kubebuilder:default:=true
	RequireHealthEndpoint bool `json:"requireHealthEndpoint"`
}

// AvtoVAZCapacityPolicy defines application capacity limits.
type AvtoVAZCapacityPolicy struct {
	// MaximumApplications is fixed to one for the Brunernetes MVP.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=1
	// +kubebuilder:default:=1
	MaximumApplications int32 `json:"maximumApplications"`
}

// AvtoVAZFridayPolicy defines explicit approval requirements for Friday changes.
type AvtoVAZFridayPolicy struct {
	// RequiredOverrideAnnotation is the annotation key used to approve Friday changes.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:default:="bruner.dev/friday-override"
	RequiredOverrideAnnotation string `json:"requiredOverrideAnnotation"`
}

// AvtoVAZRolloutPolicy defines the only supported rollout limits.
type AvtoVAZRolloutPolicy struct {
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=0
	// +kubebuilder:default:=0
	MaxUnavailable int32 `json:"maxUnavailable"`
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=1
	// +kubebuilder:default:=1
	MaxSurge int32 `json:"maxSurge"`
	// +kubebuilder:validation:Minimum=30
	// +kubebuilder:validation:Maximum=3600
	// +kubebuilder:default:=120
	ProgressDeadlineSeconds int32 `json:"progressDeadlineSeconds"`
}

// AvtoVAZStatus defines the observed policy state.
type AvtoVAZStatus struct {
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	EffectiveDefaults BrunerMode `json:"effectiveDefaults,omitempty"`
	AdmittedApplications int32 `json:"admittedApplications,omitempty"`
	MaximumApplications int32 `json:"maximumApplications,omitempty"`
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=avz
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Default Profile",type=string,JSONPath=`.status.effectiveDefaults.profile`
// +kubebuilder:printcolumn:name="Quality Gate",type=string,JSONPath=`.status.effectiveDefaults.qualityGate`
// +kubebuilder:printcolumn:name="Max Apps",type=integer,JSONPath=`.status.maximumApplications`
// +kubebuilder:printcolumn:name="Latest Tag Policy",type=string,JSONPath=`.spec.quality.latestTagBehavior`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AvtoVAZ represents factory-wide Brunernetes policy.
type AvtoVAZ struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec AvtoVAZSpec `json:"spec,omitempty"`
	Status AvtoVAZStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AvtoVAZList contains a list of AvtoVAZ resources.
type AvtoVAZList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items []AvtoVAZ `json:"items"`
}
