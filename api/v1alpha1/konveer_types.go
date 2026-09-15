package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KonveerPhase is the coarse phase of a delivery workflow.
//
// +kubebuilder:validation:Enum=Pending;Validating;Applying;RollingOut;Verifying;Succeeded;Failed
type KonveerPhase string

const (
	KonveerPending    KonveerPhase = "Pending"
	KonveerValidating KonveerPhase = "Validating"
	KonveerApplying   KonveerPhase = "Applying"
	KonveerRollingOut KonveerPhase = "RollingOut"
	KonveerVerifying  KonveerPhase = "Verifying"
	KonveerSucceeded  KonveerPhase = "Succeeded"
	KonveerFailed     KonveerPhase = "Failed"
)

// StageState is the state of a single workflow stage.
//
// +kubebuilder:validation:Enum=Pending;Running;Succeeded;Failed;Skipped
type StageState string

const (
	StagePending   StageState = "Pending"
	StageRunning   StageState = "Running"
	StageSucceeded StageState = "Succeeded"
	StageFailed    StageState = "Failed"
	StageSkipped   StageState = "Skipped"
)

// KonveerSpec describes a lightweight delivery workflow for an image that
// already exists in a registry. Brunernetes never builds container images.
type KonveerSpec struct {
	// AppRef is the name of the BrunerApp in the same namespace that this
	// workflow delivers to. The BrunerApp is created when it does not exist.
	//
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	AppRef string `json:"appRef"`

	// Image is the already-built image reference to deliver.
	//
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	Image string `json:"image"`

	// AppTemplate carries optional BrunerApp spec fields applied together with
	// the image. Fields left empty keep the current value of the BrunerApp.
	//
	// +optional
	AppTemplate *KonveerAppTemplate `json:"appTemplate,omitempty"`

	// Checks selects which policy checks run before the image is applied.
	//
	// +optional
	Checks *KonveerChecks `json:"checks,omitempty"`

	// Timeouts bounds the rollout and readiness waits.
	//
	// +optional
	Timeouts *KonveerTimeouts `json:"timeouts,omitempty"`

	// Verify configures the localhost endpoint check performed after readiness.
	//
	// +optional
	Verify *KonveerVerify `json:"verify,omitempty"`

	// Override acknowledges a guarded change, for example an image change under
	// the "friday" profile.
	//
	// +optional
	Override *AppOverride `json:"override,omitempty"`
}

// KonveerAppTemplate carries the BrunerApp fields a delivery may change.
type KonveerAppTemplate struct {
	// Replicas overrides the application replica count.
	//
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=5
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// Port overrides the container port.
	//
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +optional
	Port *int32 `json:"port,omitempty"`

	// HealthPath overrides the probe path.
	//
	// +kubebuilder:validation:Pattern=`^/.*$`
	// +optional
	HealthPath string `json:"healthPath,omitempty"`

	// Mode overrides the application operating mode.
	//
	// +optional
	Mode *BrunerMode `json:"mode,omitempty"`

	// LocalExposure overrides the loopback exposure configuration.
	//
	// +optional
	LocalExposure *LocalExposure `json:"localExposure,omitempty"`
}

// KonveerChecks selects the validation performed before delivery.
type KonveerChecks struct {
	// ImageReference validates that the image reference is well formed.
	//
	// +kubebuilder:default=true
	// +optional
	ImageReference *bool `json:"imageReference,omitempty"`

	// Policy validates the delivery against the AvtoVAZ factory policy.
	//
	// +kubebuilder:default=true
	// +optional
	Policy *bool `json:"policy,omitempty"`

	// Capacity validates that the application limit is respected.
	//
	// +kubebuilder:default=true
	// +optional
	Capacity *bool `json:"capacity,omitempty"`
}

// KonveerTimeouts bounds the waiting stages of the workflow.
type KonveerTimeouts struct {
	// RolloutSeconds bounds the Deployment rollout wait.
	//
	// +kubebuilder:default=300
	// +kubebuilder:validation:Minimum=30
	// +kubebuilder:validation:Maximum=3600
	// +optional
	RolloutSeconds int32 `json:"rolloutSeconds,omitempty"`

	// ReadinessSeconds bounds the readiness wait after the rollout completes.
	//
	// +kubebuilder:default=180
	// +kubebuilder:validation:Minimum=10
	// +kubebuilder:validation:Maximum=3600
	// +optional
	ReadinessSeconds int32 `json:"readinessSeconds,omitempty"`
}

// KonveerVerify configures the post-rollout endpoint check.
type KonveerVerify struct {
	// Enabled turns the endpoint check on. The check is skipped, not failed,
	// when the controller cannot reach the loopback endpoint of the node.
	//
	// +kubebuilder:default=true
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// Path is the HTTP path requested during verification.
	//
	// +kubebuilder:default="/healthz"
	// +kubebuilder:validation:Pattern=`^/.*$`
	// +optional
	Path string `json:"path,omitempty"`

	// ExpectedStatus is the HTTP status code that counts as success.
	//
	// +kubebuilder:default=200
	// +kubebuilder:validation:Minimum=100
	// +kubebuilder:validation:Maximum=599
	// +optional
	ExpectedStatus int32 `json:"expectedStatus,omitempty"`
}

// KonveerStageStatus reports one stage of the workflow.
type KonveerStageStatus struct {
	// Name is the stage name, for example "Validate" or "Rollout".
	Name string `json:"name"`

	// State is the stage state.
	State StageState `json:"state"`

	// Message states the technical condition first, themed wording second.
	//
	// +optional
	Message string `json:"message,omitempty"`

	// StartTime is when the stage started.
	//
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// CompletionTime is when the stage finished.
	//
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`
}

// KonveerStatus reports the observed state of the delivery workflow.
type KonveerStatus struct {
	// ObservedGeneration is the spec generation the status refers to.
	//
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Phase is the coarse workflow phase.
	//
	// +optional
	Phase KonveerPhase `json:"phase,omitempty"`

	// Stages reports every stage of the workflow in execution order.
	//
	// +optional
	// +listType=atomic
	Stages []KonveerStageStatus `json:"stages,omitempty"`

	// AppliedImage is the image written to the BrunerApp.
	//
	// +optional
	AppliedImage string `json:"appliedImage,omitempty"`

	// VerifiedEndpoint is the endpoint that answered the verification request.
	//
	// +optional
	VerifiedEndpoint string `json:"verifiedEndpoint,omitempty"`

	// StartTime is when the workflow started.
	//
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// CompletionTime is when the workflow reached a terminal phase.
	//
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// Conditions maintains Validated, RolledOut, Verified, Succeeded and
	// Degraded.
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
// +kubebuilder:resource:scope=Namespaced,shortName=kvr;line,categories=brunernetes
// +kubebuilder:printcolumn:name="App",type=string,JSONPath=`.spec.appRef`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=`.status.appliedImage`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Konveer is a lightweight delivery workflow for an existing container image.
// It validates the image reference and the factory policy, applies the
// referenced BrunerApp, waits for the rollout and readiness, and verifies the
// loopback endpoint where that is possible.
type Konveer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KonveerSpec   `json:"spec,omitempty"`
	Status KonveerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// KonveerList contains a list of Konveer resources.
type KonveerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Konveer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Konveer{}, &KonveerList{})
}
