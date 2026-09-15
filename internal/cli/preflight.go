package cli

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// CheckLevel is the severity of a preflight result.
type CheckLevel string

const (
	// CheckPass means the host satisfies the requirement.
	CheckPass CheckLevel = "OK"
	// CheckWarn means the installation can continue but something is unusual.
	CheckWarn CheckLevel = "WARN"
	// CheckFail means the installation must not continue.
	CheckFail CheckLevel = "FAIL"
)

// CheckResult is the outcome of one preflight check. Message states the
// technical condition first and adds themed wording second; Remedy tells the
// operator exactly what to do.
type CheckResult struct {
	Name    string
	Level   CheckLevel
	Message string
	Remedy  string
}

// PreflightReport is the full set of preflight results.
type PreflightReport struct {
	Results []CheckResult
}

// Add appends a result.
func (r *PreflightReport) Add(result CheckResult) { r.Results = append(r.Results, result) }

// Failed reports whether any check blocks the installation.
func (r *PreflightReport) Failed() bool {
	for _, result := range r.Results {
		if result.Level == CheckFail {
			return true
		}
	}
	return false
}

// Failures returns the blocking results.
func (r *PreflightReport) Failures() []CheckResult {
	failures := make([]CheckResult, 0, len(r.Results))
	for _, result := range r.Results {
		if result.Level == CheckFail {
			failures = append(failures, result)
		}
	}
	return failures
}

// Print writes the report.
func (r *PreflightReport) Print(printer *Printer) {
	for _, result := range r.Results {
		line := fmt.Sprintf("%-22s %s", result.Name, result.Message)
		switch result.Level {
		case CheckPass:
			printer.OK("%s", line)
		case CheckWarn:
			printer.Warn("%s", line)
		default:
			printer.Fail("%s", line)
			if result.Remedy != "" {
				printer.Info("         Remedy: %s", result.Remedy)
			}
		}
	}
}

// HostFacts are the observed properties of the host. They are collected once so
// the checks stay pure and testable.
type HostFacts struct {
	OSID              string
	OSVersion         string
	OSPrettyName      string
	Architecture      string
	CPUCores          int
	MemoryGiB         float64
	FreeDiskGiB       float64
	SwapEnabled       bool
	CgroupV2          bool
	LoadedModules     map[string]bool
	Sysctls           map[string]string
	ContainerdPresent bool
	ContainerdSocket  bool
	KubeadmPresent    bool
	KubeletPresent    bool
	HelmPresent       bool
	CNIPluginsPresent map[string]bool
	BusyPorts         []int
	Root              bool
	ClusterInstalled  bool
}

// CollectHostFacts inspects the host. It never changes anything.
func CollectHostFacts() HostFacts {
	facts := HostFacts{
		Architecture:      runtime.GOARCH,
		CPUCores:          runtime.NumCPU(),
		LoadedModules:     map[string]bool{},
		Sysctls:           map[string]string{},
		CNIPluginsPresent: map[string]bool{},
		Root:              os.Geteuid() == 0,
	}

	facts.OSID, facts.OSVersion, facts.OSPrettyName = readOSRelease("/etc/os-release")
	facts.MemoryGiB = readMemoryGiB("/proc/meminfo")
	facts.FreeDiskGiB = readFreeDiskGiB("/")
	facts.SwapEnabled = readSwapEnabled("/proc/swaps")
	facts.CgroupV2 = fileExists("/sys/fs/cgroup/cgroup.controllers")

	for _, module := range RequiredKernelModules {
		facts.LoadedModules[module] = kernelModuleLoaded(module)
	}
	for key := range RequiredSysctls {
		facts.Sysctls[key] = readSysctl(key)
	}

	facts.ContainerdPresent = binaryAvailable("containerd")
	facts.ContainerdSocket = fileExists("/run/containerd/containerd.sock")
	facts.KubeadmPresent = binaryAvailable("kubeadm")
	facts.KubeletPresent = binaryAvailable("kubelet")
	facts.HelmPresent = binaryAvailable("helm")
	for _, plugin := range RequiredCNIPlugins {
		facts.CNIPluginsPresent[plugin] = fileExists(CNIBinDir + "/" + plugin)
	}
	for _, port := range RequiredPorts {
		if portBusy(port) {
			facts.BusyPorts = append(facts.BusyPorts, port)
		}
	}
	facts.ClusterInstalled = fileExists("/etc/kubernetes/manifests/kube-apiserver.yaml")

	return facts
}

// Preflight evaluates the host facts against the requirements of a single-node
// factory. It is a pure function so it can be unit tested with synthetic facts.
func Preflight(facts HostFacts) PreflightReport {
	report := PreflightReport{}

	// Operating system and architecture.
	// The architecture may be reported either as the Go value "amd64" or as the
	// uname value "x86_64"; both name the same platform.
	if facts.Architecture != "amd64" && facts.Architecture != "x86_64" {
		report.Add(CheckResult{
			Name:    "architecture",
			Level:   CheckFail,
			Message: fmt.Sprintf("CPU architecture is %q but the MVP supports x86_64 only. The tooling for this line does not fit.", facts.Architecture),
			Remedy:  "Run Brunernetes on an x86_64 host.",
		})
	} else {
		report.Add(CheckResult{Name: "architecture", Level: CheckPass, Message: "CPU architecture is x86_64."})
	}

	switch facts.OSID {
	case "ubuntu", "debian":
		report.Add(CheckResult{
			Name:    "operating system",
			Level:   CheckPass,
			Message: fmt.Sprintf("Operating system is %s.", facts.OSPrettyName),
		})
	case "":
		report.Add(CheckResult{
			Name:    "operating system",
			Level:   CheckWarn,
			Message: "Operating system could not be identified from /etc/os-release. The installation continues unverified.",
		})
	default:
		report.Add(CheckResult{
			Name:  "operating system",
			Level: CheckWarn,
			Message: fmt.Sprintf(
				"Operating system %q is not Ubuntu or Debian, which are the tested distributions. Package installation may need manual steps. This line was tooled for a different plant.",
				facts.OSPrettyName),
		})
	}

	// Privileges.
	if facts.Root {
		report.Add(CheckResult{Name: "privileges", Level: CheckPass, Message: "Running as root, so host changes can be applied."})
	} else {
		report.Add(CheckResult{
			Name:    "privileges",
			Level:   CheckFail,
			Message: "Installer is not running as root. Host level changes cannot be applied. No badge for the restricted area.",
			Remedy:  "Run: sudo brunerctl install --preset garage",
		})
	}

	// Resources.
	if facts.CPUCores < MinCPUCores {
		report.Add(CheckResult{
			Name:    "cpu",
			Level:   CheckFail,
			Message: fmt.Sprintf("Host has %d CPU cores but %d are required. The line is understaffed.", facts.CPUCores, MinCPUCores),
			Remedy:  fmt.Sprintf("Provide a host with at least %d CPU cores.", MinCPUCores),
		})
	} else {
		report.Add(CheckResult{Name: "cpu", Level: CheckPass, Message: fmt.Sprintf("Host has %d CPU cores.", facts.CPUCores)})
	}

	if facts.MemoryGiB > 0 && facts.MemoryGiB < float64(MinMemoryGiB)*0.95 {
		report.Add(CheckResult{
			Name:    "memory",
			Level:   CheckFail,
			Message: fmt.Sprintf("Host has %.1f GiB of memory but %d GiB are required. The floor is too small for this line.", facts.MemoryGiB, MinMemoryGiB),
			Remedy:  fmt.Sprintf("Provide a host with at least %d GiB of memory.", MinMemoryGiB),
		})
	} else {
		report.Add(CheckResult{Name: "memory", Level: CheckPass, Message: fmt.Sprintf("Host has %.1f GiB of memory.", facts.MemoryGiB)})
	}

	if facts.FreeDiskGiB > 0 && facts.FreeDiskGiB < float64(MinDiskGiB) {
		report.Add(CheckResult{
			Name:    "disk",
			Level:   CheckWarn,
			Message: fmt.Sprintf("Root file system has %.1f GiB free but %d GiB are recommended. The warehouse is nearly full.", facts.FreeDiskGiB, MinDiskGiB),
			Remedy:  "Free disk space before installing, or images and etcd data may not fit.",
		})
	} else {
		report.Add(CheckResult{Name: "disk", Level: CheckPass, Message: fmt.Sprintf("Root file system has %.1f GiB free.", facts.FreeDiskGiB)})
	}

	// Swap must be off for the kubelet.
	if facts.SwapEnabled {
		report.Add(CheckResult{
			Name:    "swap",
			Level:   CheckFail,
			Message: "Swap is enabled but the kubelet requires it to be disabled. Parts cannot be stored off site.",
			Remedy:  "Run: sudo swapoff -a  and comment the swap entry in /etc/fstab",
		})
	} else {
		report.Add(CheckResult{Name: "swap", Level: CheckPass, Message: "Swap is disabled."})
	}

	// cgroup v2.
	if facts.CgroupV2 {
		report.Add(CheckResult{Name: "cgroups", Level: CheckPass, Message: "cgroup v2 is mounted."})
	} else {
		report.Add(CheckResult{
			Name:    "cgroups",
			Level:   CheckFail,
			Message: "cgroup v2 is not mounted at /sys/fs/cgroup. The kubelet and containerd cannot agree on resource accounting. The floor has no measuring equipment.",
			Remedy:  "Boot the host with systemd.unified_cgroup_hierarchy=1 and reboot.",
		})
	}

	// Kernel modules are loadable by the installer, so a missing module is a
	// warning rather than a failure.
	for _, module := range RequiredKernelModules {
		if facts.LoadedModules[module] {
			report.Add(CheckResult{Name: "module " + module, Level: CheckPass, Message: fmt.Sprintf("Kernel module %q is loaded.", module)})
		} else {
			report.Add(CheckResult{
				Name:    "module " + module,
				Level:   CheckWarn,
				Message: fmt.Sprintf("Kernel module %q is not loaded. The installer loads it. Tooling is still in the storeroom.", module),
			})
		}
	}

	// Sysctls are set by the installer, so a wrong value is a warning.
	for key, expected := range RequiredSysctls {
		actual := facts.Sysctls[key]
		switch {
		case actual == expected:
			report.Add(CheckResult{Name: "sysctl", Level: CheckPass, Message: fmt.Sprintf("Sysctl %s is %s.", key, actual)})
		case actual == "":
			report.Add(CheckResult{
				Name:    "sysctl",
				Level:   CheckWarn,
				Message: fmt.Sprintf("Sysctl %s could not be read. The installer sets it after loading br_netfilter.", key),
			})
		default:
			report.Add(CheckResult{
				Name:    "sysctl",
				Level:   CheckWarn,
				Message: fmt.Sprintf("Sysctl %s is %q but %q is required. The installer corrects it.", key, actual, expected),
			})
		}
	}

	// Container runtime.
	if facts.ContainerdPresent {
		message := "containerd is installed."
		if !facts.ContainerdSocket {
			message = "containerd is installed but its socket /run/containerd/containerd.sock is missing. The installer starts the service."
			report.Add(CheckResult{Name: "containerd", Level: CheckWarn, Message: message})
		} else {
			report.Add(CheckResult{Name: "containerd", Level: CheckPass, Message: message})
		}
	} else {
		report.Add(CheckResult{
			Name:    "containerd",
			Level:   CheckWarn,
			Message: "containerd is not installed. The installer installs it from the distribution repository. The machine shop is not equipped yet.",
		})
	}

	// Ports.
	if len(facts.BusyPorts) > 0 && !facts.ClusterInstalled {
		report.Add(CheckResult{
			Name:    "ports",
			Level:   CheckFail,
			Message: fmt.Sprintf("Control-plane ports %v are already in use by another process. Two lines cannot share one bay.", facts.BusyPorts),
			Remedy:  "Stop the process holding the port, or uninstall a previous Kubernetes installation.",
		})
	} else if len(facts.BusyPorts) > 0 {
		report.Add(CheckResult{
			Name:    "ports",
			Level:   CheckPass,
			Message: fmt.Sprintf("Control-plane ports %v are in use by the existing installation, which is expected on a rerun.", facts.BusyPorts),
		})
	} else {
		report.Add(CheckResult{Name: "ports", Level: CheckPass, Message: "All required control-plane ports are free."})
	}

	// Helm is needed for the controller installation.
	if facts.HelmPresent {
		report.Add(CheckResult{Name: "helm", Level: CheckPass, Message: "helm is installed."})
	} else {
		report.Add(CheckResult{
			Name:    "helm",
			Level:   CheckWarn,
			Message: "helm is not installed. The controller chart cannot be applied until helm is available. The paperwork office is closed.",
			Remedy:  "Install helm, for example with the official install script, then rerun the installer.",
		})
	}

	// An existing cluster means this is a rerun.
	if facts.ClusterInstalled {
		report.Add(CheckResult{
			Name:    "existing cluster",
			Level:   CheckWarn,
			Message: "A control plane is already present on this host. The installer skips kubeadm init and converges the remaining steps. The line is already tooled.",
			Remedy:  "Run brunerctl uninstall --yes first if you want a clean installation.",
		})
	}

	return report
}

// readOSRelease parses /etc/os-release.
func readOSRelease(path string) (id string, version string, pretty string) {
	file, err := os.Open(path)
	if err != nil {
		return "", "", ""
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		key, value, found := strings.Cut(scanner.Text(), "=")
		if !found {
			continue
		}
		value = strings.Trim(value, `"`)
		switch key {
		case "ID":
			id = value
		case "VERSION_ID":
			version = value
		case "PRETTY_NAME":
			pretty = value
		}
	}
	if pretty == "" {
		pretty = strings.TrimSpace(id + " " + version)
	}
	return id, version, pretty
}

// readMemoryGiB parses the total memory from /proc/meminfo.
func readMemoryGiB(path string) float64 {
	file, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 2 && fields[0] == "MemTotal:" {
			kilobytes, err := strconv.ParseFloat(fields[1], 64)
			if err != nil {
				return 0
			}
			return kilobytes / (1024 * 1024)
		}
	}
	return 0
}

// readFreeDiskGiB reports the free space of the file system holding path.
func readFreeDiskGiB(path string) float64 {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0
	}
	return float64(stat.Bavail) * float64(stat.Bsize) / (1024 * 1024 * 1024)
}

// readSwapEnabled reports whether any swap device is active.
func readSwapEnabled(path string) bool {
	content, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	return len(lines) > 1
}

// kernelModuleLoaded reports whether a module is listed in /proc/modules or is
// built into the kernel.
func kernelModuleLoaded(module string) bool {
	content, err := os.ReadFile("/proc/modules")
	if err == nil {
		for _, line := range strings.Split(string(content), "\n") {
			if strings.HasPrefix(line, module+" ") {
				return true
			}
		}
	}
	// A module compiled into the kernel exposes its parameters directory.
	return fileExists("/sys/module/" + module)
}

// readSysctl reads a kernel parameter from /proc/sys.
func readSysctl(key string) string {
	path := "/proc/sys/" + strings.ReplaceAll(key, ".", "/")
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(content))
}

// portBusy reports whether a TCP port on the loopback interface is taken.
func portBusy(port int) bool {
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	connection, err := net.DialTimeout("tcp", address, 200*time.Millisecond)
	if err != nil {
		return false
	}
	_ = connection.Close()
	return true
}

// fileExists reports whether a path exists.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
