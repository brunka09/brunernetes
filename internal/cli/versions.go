package cli

// Pinned component versions. Brunernetes installs one known-good combination
// of upstream components. Changing a value here is a deliberate, reviewable
// act: the whole point of pinning is that two hosts installed on two different
// days end up identical.
const (
	// KubernetesVersion is the pinned control-plane and kubelet version.
	KubernetesVersion = "v1.32.3"
	// KubernetesMinorRepo is the pkgs.k8s.io repository channel matching
	// KubernetesVersion.
	KubernetesMinorRepo = "v1.32"
	// KubernetesPackageVersion is the Debian package version of kubeadm,
	// kubelet and kubectl.
	KubernetesPackageVersion = "1.32.3-1.1"
	// CNIPluginsVersion is the pinned upstream containernetworking plugins
	// release providing bridge, loopback, host-local and portmap.
	CNIPluginsVersion = "v1.6.2"
	// CRIToolsVersion is the pinned crictl release.
	CRIToolsVersion = "v1.32.0"
	// PauseImage is the sandbox image containerd must agree on with kubelet.
	PauseImage = "registry.k8s.io/pause:3.10"

	// PodCIDR is the pod network of the single node.
	PodCIDR = "10.244.0.0/24"
	// ServiceCIDR is the service network of the cluster.
	ServiceCIDR = "10.96.0.0/16"
	// ContainerdSocketURL is the CRI endpoint kubeadm and the kubelet use.
	ContainerdSocketURL = "unix:///run/containerd/containerd.sock"
	// ClusterDNSDomain is the cluster DNS domain.
	ClusterDNSDomain = "cluster.local"

	// CNIConfigPath is the CNI configuration file written by the installer.
	CNIConfigPath = "/etc/cni/net.d/10-brunernetes.conflist"
	// CNIBinDir is where the CNI plugin binaries are installed.
	CNIBinDir = "/opt/cni/bin"
	// ContainerdConfigPath is the containerd configuration file.
	ContainerdConfigPath = "/etc/containerd/config.toml"
	// SysctlConfigPath is the sysctl drop-in written by the installer.
	SysctlConfigPath = "/etc/sysctl.d/99-brunernetes.conf"
	// ModulesConfigPath is the kernel module drop-in written by the installer.
	ModulesConfigPath = "/etc/modules-load.d/brunernetes.conf"
	// KubeadmConfigPath is the generated kubeadm configuration.
	KubeadmConfigPath = "/etc/brunernetes/kubeadm.yaml"
	// StateDir holds the files the installer generates.
	StateDir = "/etc/brunernetes"

	// HelmReleaseName is the Helm release of the Brunernetes controller.
	HelmReleaseName = "brunernetes"
	// HelmNamespace is the namespace of the Brunernetes controller.
	HelmNamespace = "brunernetes-system"
	// DemoNamespace is the namespace of the demo application.
	DemoNamespace = "brunernetes-demo"

	// MinCPUCores is the minimum supported CPU count.
	MinCPUCores = 2
	// MinMemoryGiB is the minimum supported memory size.
	MinMemoryGiB = 4
	// MinDiskGiB is the minimum supported free disk space on the root file
	// system.
	MinDiskGiB = 20
)

// RequiredKernelModules are loaded before kubeadm runs.
var RequiredKernelModules = []string{"overlay", "br_netfilter"}

// RequiredSysctls must hold these values for pod networking to work.
var RequiredSysctls = map[string]string{
	"net.bridge.bridge-nf-call-iptables":  "1",
	"net.bridge.bridge-nf-call-ip6tables": "1",
	"net.ipv4.ip_forward":                 "1",
}

// RequiredCNIPlugins are the plugin binaries the minimal network needs.
var RequiredCNIPlugins = []string{"bridge", "loopback", "host-local", "portmap"}

// RequiredPorts must be free before kubeadm initialises the control plane.
var RequiredPorts = []int{6443, 10250, 10259, 10257, 2379, 2380}
