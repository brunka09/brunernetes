package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CapacityProfile describes how much of the node the factory is allowed to plan
// for.
//
// +kubebuilder:validation:Enum=compact;standard;spacious
type CapacityProfile string

const (
	// CapacityCompact suits a 2 vCPU / 4 GB host.
	CapacityCompact CapacityProfile = "compact"
	// CapacityStandard suits a 4 vCPU / 8 GB host.
	CapacityStandard CapacityProfile = "standard"
	// CapacitySpacious suits anything larger.
	CapacitySpacious CapacityProfile = "spacious"
)

// TogliattiSpec describes the single-node factory environment to observe.
type TogliattiSpec struct {
	// NodeName is the node that hosts the factory. When empty the controller
	// uses the only node in the cluster.
	//
	// +optional
	NodeName string `json:"nodeName,omitempty"`

	// CapacityProfile requests a capacity profile. When empty the controller
	// derives it from the observed node capacity.
	//
	// +optional
	CapacityProfile CapacityProfile `json:"capacityProfile,omitempty"`

	// ProbeIntervalSeconds is how often the environment is re-checked.
	//
	// +kubebuilder:default=60
	// +kubebuilder:validation:Minimum=10
	// +kubebuilder:validation:Maximum=3600
	// +optional
	ProbeIntervalSeconds int32 `json:"probeIntervalSeconds,omitempty"`

	// ExpectedKubernetesVersion is the pinned control-plane version the
	// installer produced, for example "v1.32.3". A mismatch is reported but
	// does not stop the line.
	//
	// +kubebuilder:validation:Pattern=`^v[0-9]+\.[0-9]+\.[0-9]+$`
	// +optional
	ExpectedKubernetesVersion string `json:"expectedKubernetesVersion,omitempty"`

	// ExpectedRuntime is the container runtime the installer configured.
	//
	// +kubebuilder:default="containerd"
	// +kubebuilder:validation:Enum=containerd
	// +optional
	ExpectedRuntime string `json:"expectedRuntime,omitempty"`
}

// NodeResources reports capacity and allocatable resources of the node.
type NodeResources struct {
	// CPUCapacity is the total CPU capacity, for example "2".
	//
	// +optional
	CPUCapacity string `json:"cpuCapacity,omitempty"`

	// CPUAllocatable is the allocatable CPU.
	//
	// +optional
	CPUAllocatable string `json:"cpuAllocatable,omitempty"`

	// MemoryCapacity is the total memory, for example "4Gi".
	//
	// +optional
	MemoryCapacity string `json:"memoryCapacity,omitempty"`

	// MemoryAllocatable is the allocatable memory.
	//
	// +optional
	MemoryAllocatable string `json:"memoryAllocatable,omitempty"`

	// EphemeralStorageAllocatable is the allocatable ephemeral storage.
	//
	// +optional
	EphemeralStorageAllocatable string `json:"ephemeralStorageAllocatable,omitempty"`

	// Pods is the maximum number of pods the node accepts.
	//
	// +optional
	Pods int64 `json:"pods,omitempty"`
}

// TogliattiStatus reports the observed state of the factory environment.
type TogliattiStatus struct {
	// ObservedGeneration is the spec generation the status refers to.
	//
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// NodeName is the observed node.
	//
	// +optional
	NodeName string `json:"nodeName,omitempty"`

	// KubernetesVersion is the kubelet version reported by the node.
	//
	// +optional
	KubernetesVersion string `json:"kubernetesVersion,omitempty"`

	// ContainerRuntime is the runtime reported by the node, for example
	// "containerd://1.7.24".
	//
	// +optional
	ContainerRuntime string `json:"containerRuntime,omitempty"`

	// ActiveCapacityProfile is the capacity profile in use.
	//
	// +optional
	ActiveCapacityProfile CapacityProfile `json:"activeCapacityProfile,omitempty"`

	// Resources reports node capacity and allocatable resources.
	//
	// +optional
	Resources *NodeResources `json:"resources,omitempty"`

	// SchedulableForWorkloads reports whether ordinary workloads may run on the
	// control-plane node, which a single-node factory requires.
	//
	// +optional
	SchedulableForWorkloads bool `json:"schedulableForWorkloads,omitempty"`

	// LocalEndpointCapable reports whether loopback host port publishing is
	// available on this node.
	//
	// +optional
	LocalEndpointCapable bool `json:"localEndpointCapable,omitempty"`

	// LastProbeTime is when the environment was last checked.
	//
	// +optional
	LastProbeTime *metav1.Time `json:"lastProbeTime,omitempty"`

	// Conditions maintains APIHealthy, NodeReady, RuntimeAvailable,
	// CNIHealthy, LocalEndpointCapable and Available.
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
// +kubebuilder:resource:scope=Cluster,shortName=tgl;factory,categories=brunernetes
// +kubebuilder:printcolumn:name="Node",type=string,JSONPath=`.status.nodeName`
// +kubebuilder:printcolumn:name="Kubernetes",type=string,JSONPath=`.status.kubernetesVersion`
// +kubebuilder:printcolumn:name="Capacity",type=string,JSONPath=`.status.activeCapacityProfile`
// +kubebuilder:printcolumn:name="Node ready",type=string,JSONPath=`.status.conditions[?(@.type=="NodeReady")].status`
// +kubebuilder:printcolumn:name="Available",type=string,JSONPath=`.status.conditions[?(@.type=="Available")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Togliatti represents the single-node factory environment: the Kubernetes API,
// the node, the container runtime, the pod network and the resources available
// to build products on.
type Togliatti struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TogliattiSpec   `json:"spec,omitempty"`
	Status TogliattiStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// TogliattiList contains a list of Togliatti resources.
type TogliattiList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Togliatti `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Togliatti{}, &TogliattiList{})
}
