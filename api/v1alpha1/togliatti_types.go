// Copyright 2026 Julia Bruner.
// Licensed under the MIT License.

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TogliattiSpec defines the desired state of the single-node factory environment.
type TogliattiSpec struct {
	// ExpectedNodeCount is fixed to one for the Brunernetes MVP.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=1
	// +kubebuilder:default:=1
	ExpectedNodeCount int32 `json:"expectedNodeCount"`

	// Endpoint defines local endpoint constraints.
	Endpoint TogliattiEndpointSpec `json:"endpoint"`

	// CapacityProfiles defines the selected and supported local profiles.
	CapacityProfiles TogliattiCapacityProfiles `json:"capacityProfiles"`

	// Diagnostics configures periodic health collection.
	Diagnostics TogliattiDiagnosticsSpec `json:"diagnostics"`
}

// TogliattiEndpointSpec defines localhost endpoint capability constraints.
type TogliattiEndpointSpec struct {
	// LocalhostOnly requires application endpoints to bind to loopback addresses only.
	// +kubebuilder:default:=true
	LocalhostOnly bool `json:"localhostOnly"`

	// GatewayPortRange reserves non-privileged ports for local application endpoints.
	GatewayPortRange GatewayPortRange `json:"gatewayPortRange"`
}

// GatewayPortRange defines an inclusive local gateway port interval.
type GatewayPortRange struct {
	// Min is the first allowed non-privileged TCP port.
	// +kubebuilder:validation:Minimum=1024
	// +kubebuilder:validation:Maximum=65535
	Min int32 `json:"min"`

	// Max is the last allowed non-privileged TCP port.
	// +kubebuilder:validation:Minimum=1024
	// +kubebuilder:validation:Maximum=65535
	Max int32 `json:"max"`
}

// TogliattiCapacityProfiles describes active and supported Bruner profiles.
type TogliattiCapacityProfiles struct {
	// Active selects the currently active profile.
	Active BrunerProfile `json:"active"`

	// Available lists profiles supported by the local factory.
	// +kubebuilder:validation:MinItems=1
	// +listType=set
	Available []BrunerProfile `json:"available"`
}

// TogliattiDiagnosticsSpec configures health observation frequency.
type TogliattiDiagnosticsSpec struct {
	// RefreshIntervalSeconds is the controller health refresh period.
	// +kubebuilder:validation:Minimum=10
	// +kubebuilder:validation:Maximum=3600
	// +kubebuilder:default:=30
	RefreshIntervalSeconds int32 `json:"refreshIntervalSeconds"`
}

// TogliattiStatus defines the observed state of the single-node factory environment.
type TogliattiStatus struct {
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	API                APIHealthStatus `json:"api,omitempty"`
	Node               NodeHealthStatus `json:"node,omitempty"`
	Runtime            RuntimeHealthStatus `json:"runtime,omitempty"`
	CNI                CNIHealthStatus `json:"cni,omitempty"`
	Resources          FactoryResourcesStatus `json:"resources,omitempty"`
	Endpoint           EndpointCapabilityStatus `json:"endpoint,omitempty"`
	ActiveCapacityProfile BrunerProfile `json:"activeCapacityProfile,omitempty"`
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// APIHealthStatus summarizes Kubernetes API availability.
type APIHealthStatus struct {
	Healthy bool `json:"healthy,omitempty"`
	LatencyMillis int64 `json:"latencyMillis,omitempty"`
}

// NodeHealthStatus summarizes the only node in the factory.
type NodeHealthStatus struct {
	Name string `json:"name,omitempty"`
	Ready bool `json:"ready,omitempty"`
	Schedulable bool `json:"schedulable,omitempty"`
	Allocatable map[string]string `json:"allocatable,omitempty"`
}

// RuntimeHealthStatus summarizes the container runtime.
type RuntimeHealthStatus struct {
	Healthy bool `json:"healthy,omitempty"`
	Runtime string `json:"runtime,omitempty"`
	Version string `json:"version,omitempty"`
}

// CNIHealthStatus summarizes pod network capability.
type CNIHealthStatus struct {
	Healthy bool `json:"healthy,omitempty"`
	Config string `json:"config,omitempty"`
	PodCIDR string `json:"podCIDR,omitempty"`
}

// FactoryResourcesStatus summarizes allocatable and requested local capacity.
type FactoryResourcesStatus struct {
	CPUAllocatable string `json:"cpuAllocatable,omitempty"`
	CPURequested string `json:"cpuRequested,omitempty"`
	MemoryAllocatable string `json:"memoryAllocatable,omitempty"`
	MemoryRequested string `json:"memoryRequested,omitempty"`
	PodsCapacity int32 `json:"podsCapacity,omitempty"`
	PodsScheduled int32 `json:"podsScheduled,omitempty"`
}

// EndpointCapabilityStatus summarizes localhost endpoint availability.
type EndpointCapabilityStatus struct {
	Capable bool `json:"capable,omitempty"`
	ListenerAddress string `json:"listenerAddress,omitempty"`
	AvailablePorts []int32 `json:"availablePorts,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=tgl
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="API Healthy",type=boolean,JSONPath=`.status.api.healthy`
// +kubebuilder:printcolumn:name="Node Ready",type=boolean,JSONPath=`.status.node.ready`
// +kubebuilder:printcolumn:name="Runtime",type=string,JSONPath=`.status.runtime.runtime`
// +kubebuilder:printcolumn:name="CNI Healthy",type=boolean,JSONPath=`.status.cni.healthy`
// +kubebuilder:printcolumn:name="Profile",type=string,JSONPath=`.status.activeCapacityProfile`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Togliatti represents the single-node Brunernetes factory environment.
type Togliatti struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec TogliattiSpec `json:"spec,omitempty"`
	Status TogliattiStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// TogliattiList contains a list of Togliatti resources.
type TogliattiList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items []Togliatti `json:"items"`
}
