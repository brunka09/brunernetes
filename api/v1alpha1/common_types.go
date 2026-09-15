package v1alpha1

// Profile selects the operating profile of the factory.
//
// +kubebuilder:validation:Enum=garage;showroom;strict;friday
type Profile string

const (
	// ProfileGarage is the minimal local developer configuration.
	ProfileGarage Profile = "garage"
	// ProfileShowroom is the presentation-friendly configuration with verbose,
	// explicit status reporting.
	ProfileShowroom Profile = "showroom"
	// ProfileStrict requires a readiness probe and resource requests.
	ProfileStrict Profile = "strict"
	// ProfileFriday requires an explicit override for image changes and rollout
	// actions, because nobody ships to the line on a Friday without a signature.
	ProfileFriday Profile = "friday"
)

// Rollout selects the rollout strategy. Only "steady" is supported in the MVP.
//
// +kubebuilder:validation:Enum=steady
type Rollout string

// RolloutSteady performs a conservative surge-free rolling update.
const RolloutSteady Rollout = "steady"

// QualityGate selects how strictly quality checks are enforced.
//
// +kubebuilder:validation:Enum=practical;strict
type QualityGate string

const (
	// QualityGatePractical reports policy problems as warnings.
	QualityGatePractical QualityGate = "practical"
	// QualityGateStrict rejects work that fails a policy check.
	QualityGateStrict QualityGate = "strict"
)

// Shift selects the working shift of the factory. The shift only affects
// reporting verbosity and requeue cadence in the MVP.
//
// +kubebuilder:validation:Enum=day;night
type Shift string

const (
	// ShiftDay is the default shift.
	ShiftDay Shift = "day"
	// ShiftNight reduces reporting noise and slows down periodic resync.
	ShiftNight Shift = "night"
)

// LatestTagPolicy describes how a mutable ":latest" image tag is handled.
//
// +kubebuilder:validation:Enum=Ignore;Warn;Reject
type LatestTagPolicy string

const (
	// LatestTagIgnore accepts ":latest" silently.
	LatestTagIgnore LatestTagPolicy = "Ignore"
	// LatestTagWarn accepts ":latest" but raises a PolicyWarning condition.
	LatestTagWarn LatestTagPolicy = "Warn"
	// LatestTagReject refuses to roll out a ":latest" image.
	LatestTagReject LatestTagPolicy = "Reject"
)

// BrunerMode is the operating mode of the factory or of a single application.
// Every field is optional so that an application can inherit unset fields from
// the cluster-scoped AvtoVAZ policy.
type BrunerMode struct {
	// Profile selects the operating profile.
	//
	// +optional
	Profile Profile `json:"profile,omitempty"`

	// Rollout selects the rollout strategy. Only "steady" is supported.
	//
	// +optional
	Rollout Rollout `json:"rollout,omitempty"`

	// QualityGate selects how strictly quality checks are enforced.
	//
	// +optional
	QualityGate QualityGate `json:"qualityGate,omitempty"`

	// Shift selects the working shift.
	//
	// +optional
	Shift Shift `json:"shift,omitempty"`
}

// LocalExposure configures localhost-only exposure of an application.
// Brunernetes never configures public exposure: the host IP is validated to be
// a loopback address.
type LocalExposure struct {
	// Enabled requests a host port binding on the loopback interface in
	// addition to the always-created ClusterIP Service.
	//
	// +kubebuilder:default=true
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// HostIP is the loopback address the application is bound to.
	//
	// +kubebuilder:default="127.0.0.1"
	// +kubebuilder:validation:Pattern=`^127\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}$`
	// +optional
	HostIP string `json:"hostIP,omitempty"`

	// HostPort is the loopback port the application is published on.
	//
	// +kubebuilder:default=30080
	// +kubebuilder:validation:Minimum=1024
	// +kubebuilder:validation:Maximum=65535
	// +optional
	HostPort int32 `json:"hostPort,omitempty"`
}

// Condition types shared by the Brunernetes API.
const (
	// ConditionReconciling reports whether the controller is currently
	// converging the resource towards its desired state.
	ConditionReconciling = "Reconciling"
	// ConditionAvailable reports whether the resource serves its purpose.
	ConditionAvailable = "Available"
	// ConditionDegraded reports a technical fault that blocks progress.
	ConditionDegraded = "Degraded"
	// ConditionPolicyWarning reports a policy deviation that does not block
	// progress.
	ConditionPolicyWarning = "PolicyWarning"
	// ConditionAPIHealthy reports Kubernetes API server health.
	ConditionAPIHealthy = "APIHealthy"
	// ConditionNodeReady reports node readiness.
	ConditionNodeReady = "NodeReady"
	// ConditionRuntimeAvailable reports container runtime availability.
	ConditionRuntimeAvailable = "RuntimeAvailable"
	// ConditionCNIHealthy reports pod network health.
	ConditionCNIHealthy = "CNIHealthy"
	// ConditionLocalEndpointCapable reports whether the node can publish a
	// loopback endpoint.
	ConditionLocalEndpointCapable = "LocalEndpointCapable"
	// ConditionPolicyEnforced reports whether factory policy is in effect.
	ConditionPolicyEnforced = "PolicyEnforced"
	// ConditionCapacityRespected reports whether the application count is
	// within the configured maximum.
	ConditionCapacityRespected = "CapacityRespected"
	// ConditionValidated reports whether a delivery workflow passed validation.
	ConditionValidated = "Validated"
	// ConditionRolledOut reports whether a delivery workflow finished its
	// rollout.
	ConditionRolledOut = "RolledOut"
	// ConditionVerified reports whether the localhost endpoint was verified.
	ConditionVerified = "Verified"
	// ConditionSucceeded reports terminal success of a delivery workflow.
	ConditionSucceeded = "Succeeded"
)

// Common condition reasons. Reasons are machine readable; human readable text
// lives in the condition message and always states the technical condition
// before any themed wording.
const (
	ReasonReconciling         = "Reconciling"
	ReasonReconcileError      = "ReconcileError"
	ReasonAllReplicasReady    = "AllReplicasReady"
	ReasonReplicasUnavailable = "ReplicasUnavailable"
	ReasonImagePullFailure    = "ImagePullFailure"
	ReasonProbeFailure        = "ProbeFailure"
	ReasonPolicyViolation     = "PolicyViolation"
	ReasonPolicyDeviation     = "PolicyDeviation"
	ReasonOverrideRequired    = "OverrideRequired"
	ReasonCapacityExceeded    = "CapacityExceeded"
	ReasonValidationFailed    = "ValidationFailed"
	ReasonValidationPassed    = "ValidationPassed"
	ReasonRolloutTimeout      = "RolloutTimeout"
	ReasonEndpointUnverified  = "EndpointUnverified"
	ReasonEndpointVerified    = "EndpointVerified"
	ReasonProbeSucceeded      = "ProbeSucceeded"
	ReasonProbeFailed         = "ProbeFailed"
)

// AssemblyStage is the themed progress stage of a workload. It mirrors the
// technical status and is provided for human readable reporting only.
//
// +kubebuilder:validation:Enum=Inspection;Assembly;QualityControl;Showroom;LineStopped
type AssemblyStage string

const (
	// StageInspection means the desired state is being validated.
	StageInspection AssemblyStage = "Inspection"
	// StageAssembly means workloads are being created or updated.
	StageAssembly AssemblyStage = "Assembly"
	// StageQualityControl means replicas exist but are not all ready.
	StageQualityControl AssemblyStage = "QualityControl"
	// StageShowroom means the application is fully available.
	StageShowroom AssemblyStage = "Showroom"
	// StageLineStopped means a fault blocks progress.
	StageLineStopped AssemblyStage = "LineStopped"
)
