package cli

import (
	"strings"
	"testing"
)

// goodHost returns host facts that satisfy every preflight check, so each test
// only has to break the one condition it is about.
func goodHost(mutators ...func(*HostFacts)) HostFacts {
	facts := HostFacts{
		OSID:              "ubuntu",
		OSVersion:         "24.04",
		OSPrettyName:      "Ubuntu 24.04.1 LTS",
		Architecture:      "x86_64",
		CPUCores:          4,
		MemoryGiB:         8,
		FreeDiskGiB:       40,
		SwapEnabled:       false,
		CgroupV2:          true,
		LoadedModules:     map[string]bool{"br_netfilter": true, "overlay": true},
		Sysctls:           map[string]string{"net.ipv4.ip_forward": "1", "net.bridge.bridge-nf-call-iptables": "1"},
		ContainerdPresent: true,
		ContainerdSocket:  true,
		KubeadmPresent:    true,
		KubeletPresent:    true,
		HelmPresent:       true,
		CNIPluginsPresent: map[string]bool{"bridge": true, "host-local": true, "loopback": true, "portmap": true},
		BusyPorts:         nil,
		Root:              true,
		ClusterInstalled:  false,
	}
	for _, mutate := range mutators {
		mutate(&facts)
	}
	return facts
}

// findCheck returns the result of one named check.
func findCheck(report PreflightReport, name string) (CheckResult, bool) {
	for _, result := range report.Results {
		if result.Name == name {
			return result, true
		}
	}
	return CheckResult{}, false
}

func TestPreflightAcceptsASupportedHost(t *testing.T) {
	report := Preflight(goodHost())
	if report.Failed() {
		t.Fatalf("a supported host must pass preflight: %+v", report.Failures())
	}
	if len(report.Results) == 0 {
		t.Fatal("preflight must report the checks it ran")
	}
	for _, result := range report.Results {
		if result.Message == "" {
			t.Errorf("check %q produced no message", result.Name)
		}
	}
}

func TestPreflightBlocksAnUnsupportedHost(t *testing.T) {
	cases := []struct {
		name   string
		break_ func(*HostFacts)
		check  string
	}{
		{
			name:   "a 32-bit or arm host is not supported",
			break_: func(f *HostFacts) { f.Architecture = "aarch64" },
			check:  "architecture",
		},
		{
			name:   "installation needs root",
			break_: func(f *HostFacts) { f.Root = false },
			check:  "privileges",
		},
		{
			name:   "one CPU core is not enough for a control plane",
			break_: func(f *HostFacts) { f.CPUCores = 1 },
			check:  "cpu",
		},
		{
			name:   "two gigabytes of memory is not enough",
			break_: func(f *HostFacts) { f.MemoryGiB = 2 },
			check:  "memory",
		},
		{
			name:   "swap must be disabled for the kubelet",
			break_: func(f *HostFacts) { f.SwapEnabled = true },
			check:  "swap",
		},
		{
			name:   "cgroup v2 is required",
			break_: func(f *HostFacts) { f.CgroupV2 = false },
			check:  "cgroups",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			report := Preflight(goodHost(testCase.break_))
			if !report.Failed() {
				t.Fatalf("check %q was expected to fail", testCase.check)
			}
			result, found := findCheck(report, testCase.check)
			if !found {
				t.Fatalf("check %q is missing from the report", testCase.check)
			}
			if result.Level != CheckFail {
				t.Errorf("check %q: got level %q, want %q", testCase.check, result.Level, CheckFail)
			}
			if result.Remedy == "" {
				t.Errorf("a blocking check must tell the operator what to do: %+v", result)
			}
		})
	}
}

func TestPreflightWarnsWithoutBlocking(t *testing.T) {
	cases := []struct {
		name   string
		break_ func(*HostFacts)
		check  string
	}{
		{
			name:   "a nearly full disk only warns",
			break_: func(f *HostFacts) { f.FreeDiskGiB = 5 },
			check:  "disk",
		},
		{
			name:   "an untested distribution warns",
			break_: func(f *HostFacts) { f.OSID = "fedora"; f.OSPrettyName = "Fedora 41" },
			check:  "operating system",
		},
		{
			name:   "a missing kernel module is loaded by the installer",
			break_: func(f *HostFacts) { f.LoadedModules = map[string]bool{} },
			check:  "module br_netfilter",
		},
		{
			name:   "a missing helm binary only warns",
			break_: func(f *HostFacts) { f.HelmPresent = false },
			check:  "helm",
		},
		{
			name:   "a missing containerd is installed by the installer",
			break_: func(f *HostFacts) { f.ContainerdPresent = false },
			check:  "containerd",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			report := Preflight(goodHost(testCase.break_))
			result, found := findCheck(report, testCase.check)
			if !found {
				t.Fatalf("check %q is missing from the report", testCase.check)
			}
			if result.Level != CheckWarn {
				t.Errorf("check %q: got level %q, want %q", testCase.check, result.Level, CheckWarn)
			}
			if report.Failed() {
				t.Errorf("a warning must not block the installation: %+v", report.Failures())
			}
		})
	}
}

func TestPreflightReportsAnExistingCluster(t *testing.T) {
	report := Preflight(goodHost(func(f *HostFacts) { f.ClusterInstalled = true }))
	result, found := findCheck(report, "existing cluster")
	if !found {
		t.Fatal("an existing cluster must be reported")
	}
	if result.Level == CheckPass {
		t.Errorf("an existing cluster must at least warn: %+v", result)
	}
}

func TestPreflightReportsABusyPort(t *testing.T) {
	report := Preflight(goodHost(func(f *HostFacts) { f.BusyPorts = []int{6443} }))
	result, found := findCheck(report, "ports")
	if !found {
		t.Fatal("busy ports must be reported")
	}
	if result.Level != CheckFail {
		t.Errorf("a busy API server port must block the installation: %+v", result)
	}
	if !strings.Contains(result.Message, "6443") {
		t.Errorf("the message must name the busy port: %q", result.Message)
	}
}

func TestPreflightMessagesStateTheConditionFirst(t *testing.T) {
	report := Preflight(goodHost(func(f *HostFacts) { f.CPUCores = 1; f.MemoryGiB = 1 }))
	for _, result := range report.Results {
		if strings.HasPrefix(result.Message, "The factory") || strings.HasPrefix(result.Message, "The line") {
			t.Errorf("check %q opens with themed wording: %q", result.Name, result.Message)
		}
	}
}
