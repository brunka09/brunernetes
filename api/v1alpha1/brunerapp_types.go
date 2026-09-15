package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BrunerAppSpec describes the single web application produced by the factory.
type BrunerAppSpec struct {
	// Image is the container image reference of an already-built image.
	// Brunernetes never builds images.
	//
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	Image string `json:"image"`

	// ImagePullPolicy overrides the container image pull policy.
	//
	// +kubebuilder:validation:Enum=Always;IfNotPresent;Never
	// +optional
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`

	// ImagePullSecrets references registry credentials in the same namespace.
	//
	// +optional
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`

	// Replicas is the desired replica count. A single node runs a small number
	// of replicas, so the maximum is deliberately low.
	//
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=5
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// Port is the container port serving HTTP traffic.
	//
	// +kubebuilder:default=8080
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +optional
	Port int32 `json:"port,omitempty"`

	// HealthPath is the HTTP path used for the default probes.
	//
	// +kubebuilder:default="/healthz"
	// +kubebuilder:validation:Pattern=`^/.*$`
	// +optional
	HealthPath string `json:"healthPath,omitempty"`

	// Mode overrides the factory default operating mode for this application.
	//
	// +optional
	Mode *BrunerMode `json:"mode,omitempty"`

	// LocalExposure configures the loopback-only host port binding.
	//
	// +optional
	LocalExposure *LocalExposure `json:"localExposure,omitempty"`

	// Env are additional environment variables for the container.
	//
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`

	// Resources overrides the default resource requests and limits. Fields left
	// empty are filled with factory defaults.
	//
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// ReadinessProbe overrides the default readiness probe.
	//
	// +optional
	ReadinessProbe *corev1.Probe `json:"readinessProbe,omitempty"`

	// LivenessProbe overrides the default liveness probe.
	//
	// +optional
	LivenessProbe *corev1.Probe `json:"livenessProbe,omitempty"`

	// ServiceAccountName is the service account used by the application pods.
	//
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`

	// Override acknowledges a change that the active profile would otherwise
	// hold back, for example an image change under the "friday" profile. The
	// value must be changed by a human for each guarded change.
	//
	// +optional
	Override *AppOverride `json:"override,omitempty"`
}

// AppOverride is an explicit human acknowledgement of a guarded change.
type AppOverride struct {
	// Approved enables the guarded change.
	Approved bool `json:"approved"`

	// Image is the exact image reference the approval applies to. The approval
	// is ignored when it does not match spec.image.
	//
	// +optional
	Image string `json:"image,omitempty"`

	// Reason documents why the change was approved.
	//
	// +kubebuilder:validation:MaxLength=256
	// +optional
	Reason string `json:"reason,omitempty"`
}

// BrunerAppStatus reports the observed state of the application.
type BrunerAppStatus struct {
	// ObservedGeneration is the spec generation the status refers to.
	//
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Stage is the themed progress stage derived from the technical status.
	//
	// +optional
	Stage AssemblyStage `json:"stage,omitempty"`

	// Replicas is the number of replicas currently created.
	//
	// +optional
	Replicas int32 `json:"replicas,omitempty"`

	// ReadyReplicas is the number of replicas that passed readiness.
	//
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// DeploymentName is the owned Deployment.
	//
	// +optional
	DeploymentName string `json:"deploymentName,omitempty"`

	// ServiceName is the owned Service.
	//
	// +optional
	ServiceName string `json:"serviceName,omitempty"`

	// ClusterIPEndpoint is the in-cluster endpoint of the Service.
	//
	// +optional
	ClusterIPEndpoint string `json:"clusterIPEndpoint,omitempty"`

	// LocalEndpoint is the loopback endpoint on the node, when local exposure
	// is enabled.
	//
	// +optional
	LocalEndpoint string `json:"localEndpoint,omitempty"`

	// AppliedImage is the image currently rolled out.
	//
	// +optional
	AppliedImage string `json:"appliedImage,omitempty"`

	// EffectiveMode is the mode after merging the AvtoVAZ defaults with the
	// application override.
	//
	// +optional
	EffectiveMode *BrunerMode `json:"effectiveMode,omitempty"`

	// Conditions follow the standard Kubernetes condition contract. The
	// controller maintains Reconciling, Available, Degraded and PolicyWarning.
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
// +kubebuilder:resource:scope=Namespaced,shortName=bapp;app,categories=brunernetes
// +kubebuilder:printcolumn:name="Stage",type=string,JSONPath=`.status.stage`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.readyReplicas`
// +kubebuilder:printcolumn:name="Replicas",type=string,JSONPath=`.status.replicas`
// +kubebuilder:printcolumn:name="Local endpoint",type=string,JSONPath=`.status.localEndpoint`
// +kubebuilder:printcolumn:name="Available",type=string,JSONPath=`.status.conditions[?(@.type=="Available")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// BrunerApp is the single web application assembled and published by the
// Brunernetes factory. The controller owns one Deployment and one Service and
// exposes the application on the loopback interface only.
type BrunerApp struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BrunerAppSpec   `json:"spec,omitempty"`
	Status BrunerAppStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BrunerAppList contains a list of BrunerApp resources.
type BrunerAppList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BrunerApp `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BrunerApp{}, &BrunerAppList{})
}
