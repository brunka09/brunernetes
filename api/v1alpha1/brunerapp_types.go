// Copyright 2026 Julia Bruner.
// Licensed under the MIT License.

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LocalExposureType defines supported application exposure mechanisms.
// +kubebuilder:validation:Enum=Localhost
type LocalExposureType string

const LocalExposureTypeLocalhost LocalExposureType = "Localhost"

// BrunerAppPhase defines observed application lifecycle phases.
// +kubebuilder:validation:Enum=Pending;Validating;Progressing;Ready;Degraded;Rejected
type BrunerAppPhase string

const (
	BrunerAppPhasePending BrunerAppPhase = "Pending"
	BrunerAppPhaseValidating BrunerAppPhase = "Validating"
	BrunerAppPhaseProgressing BrunerAppPhase = "Progressing"
	BrunerAppPhaseReady BrunerAppPhase = "Ready"
	BrunerAppPhaseDegraded BrunerAppPhase = "Degraded"
	BrunerAppPhaseRejected BrunerAppPhase = "Rejected"
)

// BrunerAppSpec defines the desired state of one local web application.
type BrunerAppSpec struct {
	// Image is an OCI-style image reference for the application.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	Image string `json:"image"`

	// Port is the application container TCP port.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port"`

	// Replicas is fixed to one for the Brunernetes MVP.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=1
	// +kubebuilder:default:=1
	Replicas int32 `json:"replicas"`

	BrunerMode BrunerMode `json:"brunerMode"`
	Exposure BrunerAppExposureSpec `json:"exposure"`
	Health BrunerAppHealthSpec `json:"health"`
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
	Rollout BrunerAppRolloutSpec `json:"rollout"`
	// +optional
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
}

// BrunerAppExposureSpec defines localhost-only application exposure.
type BrunerAppExposureSpec struct {
	Type LocalExposureType `json:"type"`
	// +kubebuilder:validation:Minimum=1024
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port"`
}

// BrunerAppHealthSpec defines HTTP health probing behavior.
type BrunerAppHealthSpec struct {
	// Path is the absolute HTTP health endpoint path.
	// +kubebuilder:validation:Pattern=`^/.*`
	// +kubebuilder:validation:MaxLength=256
	Path string `json:"path"`
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=300
	// +kubebuilder:default:=2
	InitialDelaySeconds int32 `json:"initialDelaySeconds"`
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=300
	// +kubebuilder:default:=5
	PeriodSeconds int32 `json:"periodSeconds"`
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=60
	// +kubebuilder:default:=2
	TimeoutSeconds int32 `json:"timeoutSeconds"`
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=20
	// +kubebuilder:default:=6
	FailureThreshold int32 `json:"failureThreshold"`
}

// BrunerAppRolloutSpec defines the supported application rollout strategy.
type BrunerAppRolloutSpec struct {
	// +kubebuilder:validation:Enum=RollingUpdate
	// +kubebuilder:default:=RollingUpdate
	Strategy string `json:"strategy"`
}

// BrunerAppStatus defines the observed application state.
type BrunerAppStatus struct {
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	Phase BrunerAppPhase `json:"phase,omitempty"`
	EffectiveBrunerMode BrunerMode `json:"effectiveBrunerMode,omitempty"`
	Deployment BrunerAppDeploymentStatus `json:"deployment,omitempty"`
	Service BrunerAppServiceStatus `json:"service,omitempty"`
	Endpoint BrunerAppEndpointStatus `json:"endpoint,omitempty"`
	Image BrunerAppImageStatus `json:"image,omitempty"`
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// BrunerAppDeploymentStatus summarizes the managed deployment.
type BrunerAppDeploymentStatus struct {
	Name string `json:"name,omitempty"`
	Generation int64 `json:"generation,omitempty"`
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`
	UpdatedReplicas int32 `json:"updatedReplicas,omitempty"`
	AvailableReplicas int32 `json:"availableReplicas,omitempty"`
}

// BrunerAppServiceStatus summarizes the managed ClusterIP service.
type BrunerAppServiceStatus struct {
	Name string `json:"name,omitempty"`
	ClusterIP string `json:"clusterIP,omitempty"`
}

// BrunerAppEndpointStatus summarizes the localhost endpoint.
type BrunerAppEndpointStatus struct {
	URL string `json:"url,omitempty"`
	Verified bool `json:"verified,omitempty"`
	LastVerifiedAt *metav1.Time `json:"lastVerifiedAt,omitempty"`
}

// BrunerAppImageStatus summarizes the requested and resolved image.
type BrunerAppImageStatus struct {
	Reference string `json:"reference,omitempty"`
	ResolvedDigest string `json:"resolvedDigest,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,shortName=bapp
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Ready",type=integer,JSONPath=`.status.deployment.readyReplicas`
// +kubebuilder:printcolumn:name="Endpoint",type=string,JSONPath=`.status.endpoint.url`
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=`.spec.image`
// +kubebuilder:printcolumn:name="Profile",type=string,JSONPath=`.status.effectiveBrunerMode.profile`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// BrunerApp represents one locally exposed Brunernetes application.
type BrunerApp struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec BrunerAppSpec `json:"spec,omitempty"`
	Status BrunerAppStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BrunerAppList contains a list of BrunerApp resources.
type BrunerAppList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items []BrunerApp `json:"items"`
}
