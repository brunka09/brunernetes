package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AvtoVAZSpec describes factory-wide configuration and policy.
type AvtoVAZSpec struct {
	// DefaultMode is the operating mode inherited by every application that
	// does not set its own mode fields.
	//
	// +optional
	DefaultMode *BrunerMode `json:"defaultMode,omitempty"`

	// Policy is the factory safety and quality policy.
	//
	// +optional
	Policy *FactoryPolicy `json:"policy,omitempty"`

	// FactoryRef is the name of the Togliatti resource this policy applies to.
	//
	// +kubebuilder:default="togliatti"
	// +optional
	FactoryRef string `json:"factoryRef,omitempty"`
}

// FactoryPolicy is the safety and quality policy of the factory.
type FactoryPolicy struct {
	// RequireReadinessProbe rejects or warns about applications without a
	// readiness probe, depending on the active quality gate.
	//
	// +kubebuilder:default=false
	// +optional
	RequireReadinessProbe *bool `json:"requireReadinessProbe,omitempty"`

	// RequireResourceRequests rejects or warns about applications without CPU
	// and memory requests, depending on the active quality gate.
	//
	// +kubebuilder:default=false
	// +optional
	RequireResourceRequests *bool `json:"requireResourceRequests,omitempty"`

	// LatestTagPolicy describes how a mutable ":latest" image tag is handled.
	//
	// +kubebuilder:default=Warn
	// +optional
	LatestTagPolicy LatestTagPolicy `json:"latestTagPolicy,omitempty"`

	// MaxApplications is the maximum number of BrunerApp resources allowed in
	// the cluster. The MVP supports exactly one application.
	//
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=1
	// +optional
	MaxApplications int32 `json:"maxApplications,omitempty"`

	// AllowedRegistries restricts image references to the listed registry
	// prefixes. An empty list allows any registry.
	//
	// +optional
	AllowedRegistries []string `json:"allowedRegistries,omitempty"`

	// AllowPublicExposure is always false in the MVP and exists to make the
	// localhost-only guarantee explicit and auditable.
	//
	// +kubebuilder:default=false
	// +kubebuilder:validation:Enum=false
	// +optional
	AllowPublicExposure *bool `json:"allowPublicExposure,omitempty"`
}

// AvtoVAZStatus reports the observed policy state.
type AvtoVAZStatus struct {
	// ObservedGeneration is the spec generation the status refers to.
	//
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// EffectiveMode is the fully defaulted factory mode.
	//
	// +optional
	EffectiveMode *BrunerMode `json:"effectiveMode,omitempty"`

	// ApplicationCount is the number of BrunerApp resources observed.
	//
	// +optional
	ApplicationCount int32 `json:"applicationCount,omitempty"`

	// MaxApplications is the enforced application limit.
	//
	// +optional
	MaxApplications int32 `json:"maxApplications,omitempty"`

	// Conditions maintains PolicyEnforced and CapacityRespected.
	//
	// +optional
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=vaz,categories=brunernetes
// +kubebuilder:printcolumn:name="Profile",type=string,JSONPath=`.status.effectiveMode.profile`
// +kubebuilder:printcolumn:name="Quality gate",type=string,JSONPath=`.status.effectiveMode.qualityGate`
// +kubebuilder:printcolumn:name="Apps",type=string,JSONPath=`.status.applicationCount`
// +kubebuilder:printcolumn:name="Max apps",type=string,JSONPath=`.status.maxApplications`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AvtoVAZ is the factory-wide configuration and policy resource. It defines the
// default operating mode and the safety and quality rules every application in
// the factory is measured against.
type AvtoVAZ struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AvtoVAZSpec   `json:"spec,omitempty"`
	Status AvtoVAZStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AvtoVAZList contains a list of AvtoVAZ resources.
type AvtoVAZList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AvtoVAZ `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AvtoVAZ{}, &AvtoVAZList{})
}
