package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	brunerv1alpha1 "github.com/brunka09/brunernetes/api/v1alpha1"
)

// InstallOptions configures the installer.
type InstallOptions struct {
	// Preset selects the operating profile written into the AvtoVAZ resource.
	Preset string
	// DryRun prints every host change instead of applying it.
	DryRun bool
	// SkipPreflight continues even when a blocking check fails. It exists for
	// unusual hosts and is never the recommended path.
	SkipPreflight bool
	// SkipPackages assumes kubeadm, kubelet, kubectl and containerd are already
	// installed at the pinned versions.
	SkipPackages bool
	// SkipCNI leaves the CNI configuration untouched.
	SkipCNI bool
	// SkipController does not install the Helm chart.
	SkipController bool
	// ChartPath is the local Helm chart directory.
	ChartPath string
	// KubernetesVersion overrides the pinned Kubernetes version.
	KubernetesVersion string
	// PodCIDR overrides the pod network.
	PodCIDR string
	// APIAdvertiseAddress overrides the API server advertise address.
	APIAdvertiseAddress string
}

// Installer performs the single-node installation.
type Installer struct {
	Printer *Printer
	Runner  CommandRunner
	Options InstallOptions
}

// totalSteps is the number of installation steps reported to the operator.
const totalSteps = 9

// Install runs the whole installation. Every step is safe to repeat: the
// installer checks the current state of the host before it changes anything.
func (i *Installer) Install(ctx context.Context) error {
	printer := i.Printer
	options := i.Options

	version := options.KubernetesVersion
	if version == "" {
		version = KubernetesVersion
	}
	podCIDR := options.PodCIDR
	if podCIDR == "" {
		podCIDR = PodCIDR
	}

	printer.Section("Brunernetes installation")
	printer.KeyValues([][2]string{
		{"Preset", options.Preset},
		{"Kubernetes version", version},
		{"Container runtime", "containerd"},
		{"Pod network", podCIDR},
		{"Service network", ServiceCIDR},
		{"Mode", installMode(options.DryRun)},
	})

	// Step 1: preflight.
	printer.Step(1, totalSteps, "Checking the host before anything is changed.")
	facts := CollectHostFacts()
	if options.DryRun {
		// A dry run is useful for a non-root operator, so the privilege check
		// must not make the report look broken.
		facts.Root = true
	}
	report := Preflight(facts)
	report.Print(printer)
	if report.Failed() {
		if !options.SkipPreflight {
			printer.Fail("Preflight checks failed. The installation was not started. The factory gate stays closed.")
			for _, failure := range report.Failures() {
				printer.Info("- %s", failure.Message)
			}
			return fmt.Errorf("preflight checks failed: %d blocking problems", len(report.Failures()))
		}
		printer.Warn("Preflight checks failed but --skip-preflight was given. Continuing on an unverified host.")
	}

	// Step 2: kernel modules.
	printer.Step(2, totalSteps, "Loading the kernel modules the pod network needs.")
	moduleConfig := strings.Join(RequiredKernelModules, "\n") + "\n"
	if err := i.Runner.WriteFile(ModulesConfigPath, moduleConfig, 0o644); err != nil {
		return err
	}
	for _, module := range RequiredKernelModules {
		if _, err := i.Runner.Run(ctx, "modprobe", module); err != nil {
			printer.Warn("Kernel module %q could not be loaded: %v. It may be built into the kernel.", module, err)
		} else {
			printer.OK("Kernel module %q is loaded.", module)
		}
	}

	// Step 3: sysctls.
	printer.Step(3, totalSteps, "Applying the sysctl values required for pod networking.")
	sysctlLines := make([]string, 0, len(RequiredSysctls))
	for _, key := range sortedSysctlKeys() {
		sysctlLines = append(sysctlLines, fmt.Sprintf("%s = %s", key, RequiredSysctls[key]))
	}
	if err := i.Runner.WriteFile(SysctlConfigPath, strings.Join(sysctlLines, "\n")+"\n", 0o644); err != nil {
		return err
	}
	if _, err := i.Runner.Run(ctx, "sysctl", "--system"); err != nil {
		printer.Warn("Sysctl values could not be reloaded: %v. Apply them manually with sysctl --system.", err)
	} else {
		printer.OK("Sysctl values are applied.")
	}

	// Step 4: packages and container runtime.
	printer.Step(4, totalSteps, "Installing the pinned Kubernetes packages and the container runtime.")
	if options.SkipPackages {
		printer.Skip("Package installation was skipped by request.")
	} else if err := i.installPackages(ctx, facts); err != nil {
		return err
	}

	// Step 5: containerd configuration.
	printer.Step(5, totalSteps, "Configuring containerd for the systemd cgroup driver.")
	if options.SkipPackages && facts.ContainerdPresent {
		printer.Skip("containerd configuration was skipped together with package installation.")
	} else if err := i.configureContainerd(ctx); err != nil {
		return err
	}

	// Step 6: control plane.
	printer.Step(6, totalSteps, "Bootstrapping the control plane with kubeadm.")
	if facts.ClusterInstalled {
		printer.Skip("A control plane already exists on this host, so kubeadm init is not run again.")
	} else {
		if err := i.bootstrapControlPlane(ctx, version, podCIDR); err != nil {
			return err
		}
	}

	// Step 7: single-node scheduling and CNI.
	printer.Step(7, totalSteps, "Preparing the node to run workloads and configuring the pod network.")
	if err := i.prepareNode(ctx, podCIDR); err != nil {
		return err
	}

	// Step 8: controller.
	printer.Step(8, totalSteps, "Installing the Brunernetes CRDs and controller with Helm.")
	if options.SkipController {
		printer.Skip("Controller installation was skipped by request.")
	} else if err := i.installController(ctx); err != nil {
		return err
	}

	// Step 9: factory resources.
	printer.Step(9, totalSteps, "Creating the factory resources for the selected preset.")
	if err := i.applyFactoryResources(ctx); err != nil {
		return err
	}

	printer.Section("Installation complete")
	if options.DryRun {
		printer.Println("Dry run finished. No change was applied to this host. The plan is on the desk, the line has not moved.")
		return nil
	}
	printer.Println("Single-node Kubernetes is installed and the Brunernetes controller is running. The factory is open.")
	printer.Info("Next: brunerctl status")
	printer.Info("Then: brunerctl app deploy -f config/samples/bruner.dev_v1alpha1_brunerapp.yaml")
	return nil
}

// installPackages installs containerd and the pinned Kubernetes packages from
// the official pkgs.k8s.io repository.
func (i *Installer) installPackages(ctx context.Context, facts HostFacts) error {
	printer := i.Printer

	if facts.OSID != "ubuntu" && facts.OSID != "debian" && facts.OSID != "" {
		printer.Warn("Distribution %q is not Debian based, so package installation is skipped. Install containerd, kubeadm %s, kubelet and kubectl manually.",
			facts.OSPrettyName, KubernetesPackageVersion)
		return nil
	}

	repoLine := fmt.Sprintf(
		"deb [signed-by=/etc/apt/keyrings/kubernetes-apt-keyring.gpg] https://pkgs.k8s.io/core:/stable:/%s/deb/ /\n",
		KubernetesMinorRepo)

	steps := []struct {
		description string
		run         func() error
	}{
		{"update the package index", func() error {
			_, err := i.Runner.Run(ctx, "apt-get", "update")
			return err
		}},
		{"install the packages needed to add a repository", func() error {
			_, err := i.Runner.Run(ctx, "apt-get", "install", "-y", "apt-transport-https", "ca-certificates", "curl", "gpg", "containerd")
			return err
		}},
		{"add the Kubernetes package signing key", func() error {
			script := fmt.Sprintf(
				"install -m 0755 -d /etc/apt/keyrings && "+
					"curl -fsSL https://pkgs.k8s.io/core:/stable:/%s/deb/Release.key | "+
					"gpg --batch --yes --dearmor -o /etc/apt/keyrings/kubernetes-apt-keyring.gpg && "+
					"chmod 0644 /etc/apt/keyrings/kubernetes-apt-keyring.gpg",
				KubernetesMinorRepo)
			_, err := i.Runner.Run(ctx, "bash", "-c", script)
			return err
		}},
		{"add the Kubernetes package repository", func() error {
			return i.Runner.WriteFile("/etc/apt/sources.list.d/kubernetes.list", repoLine, 0o644)
		}},
		{"install the pinned Kubernetes packages", func() error {
			_, err := i.Runner.Run(ctx, "bash", "-c", fmt.Sprintf(
				"apt-get update && apt-get install -y kubelet=%s kubeadm=%s kubectl=%s && apt-mark hold kubelet kubeadm kubectl",
				KubernetesPackageVersion, KubernetesPackageVersion, KubernetesPackageVersion))
			return err
		}},
	}

	for _, step := range steps {
		if err := step.run(); err != nil {
			return fmt.Errorf(
				"package installation failed while trying to %s: %w. Check network access to the distribution and Kubernetes repositories. Parts delivery to the plant failed",
				step.description, err)
		}
		printer.OK("Completed: %s.", step.description)
	}
	return nil
}

// configureContainerd writes a containerd configuration that uses the systemd
// cgroup driver and the pinned pause image, then restarts the service.
func (i *Installer) configureContainerd(ctx context.Context) error {
	printer := i.Printer

	output, err := i.Runner.Run(ctx, "bash", "-c", "containerd config default")
	if err != nil {
		printer.Warn("Default containerd configuration could not be generated: %v. The existing configuration is kept.", err)
	} else if output != "" {
		configured := strings.ReplaceAll(output, "SystemdCgroup = false", "SystemdCgroup = true")
		configured = replacePauseImage(configured, PauseImage)
		if err := i.Runner.WriteFile(ContainerdConfigPath, configured, 0o644); err != nil {
			return err
		}
		printer.OK("containerd configuration written to %s with SystemdCgroup enabled.", ContainerdConfigPath)
	} else {
		// Dry run: report the intent without writing anything.
		if err := i.Runner.WriteFile(ContainerdConfigPath, "", 0o644); err != nil {
			return err
		}
	}

	if _, err := i.Runner.Run(ctx, "systemctl", "restart", "containerd"); err != nil {
		return fmt.Errorf(
			"containerd could not be restarted: %w. Run 'systemctl status containerd' and 'journalctl -u containerd' to see why. The machine shop did not start", err)
	}
	if _, err := i.Runner.Run(ctx, "systemctl", "enable", "containerd"); err != nil {
		printer.Warn("containerd could not be enabled at boot: %v.", err)
	}
	printer.OK("containerd is running with the systemd cgroup driver.")
	return nil
}

// bootstrapControlPlane generates a kubeadm configuration and initialises the
// single-node control plane.
func (i *Installer) bootstrapControlPlane(ctx context.Context, version string, podCIDR string) error {
	printer := i.Printer

	config := KubeadmConfig(version, podCIDR, i.Options.APIAdvertiseAddress)
	if err := i.Runner.WriteFile(KubeadmConfigPath, config, 0o600); err != nil {
		return err
	}
	printer.OK("kubeadm configuration written to %s.", KubeadmConfigPath)

	if _, err := i.Runner.Run(ctx, "kubeadm", "init", "--config", KubeadmConfigPath); err != nil {
		return fmt.Errorf(
			"kubeadm init failed: %w. Run 'kubeadm reset -f' before retrying and inspect 'journalctl -u kubelet'. The control-plane building did not open", err)
	}
	printer.OK("Control plane is initialised.")

	// Make the admin kubeconfig usable by the invoking user.
	if _, err := i.Runner.Run(ctx, "bash", "-c",
		"mkdir -p \"$HOME/.kube\" && cp -f /etc/kubernetes/admin.conf \"$HOME/.kube/config\" && "+
			"chown \"$(id -u)\":\"$(id -g)\" \"$HOME/.kube/config\""); err != nil {
		printer.Warn("Admin kubeconfig could not be copied to the home directory: %v. Use KUBECONFIG=%s instead.", err, AdminKubeconfigPath)
	} else {
		printer.OK("Admin kubeconfig copied to $HOME/.kube/config.")
	}
	return nil
}

// prepareNode removes the control-plane taint so the single node runs workloads
// and installs the minimal CNI configuration.
func (i *Installer) prepareNode(ctx context.Context, podCIDR string) error {
	printer := i.Printer

	kubectlEnv := "KUBECONFIG=" + AdminKubeconfigPath
	if _, err := i.Runner.Run(ctx, "bash", "-c",
		kubectlEnv+" kubectl taint nodes --all node-role.kubernetes.io/control-plane- --overwrite=false || true"); err != nil {
		printer.Warn("Control-plane taint could not be removed: %v. Workloads will stay Pending until it is removed.", err)
	} else {
		printer.OK("Control-plane taint removed, so the node runs workloads.")
	}

	if i.Options.SkipCNI {
		printer.Skip("CNI configuration was skipped by request.")
		return nil
	}

	missing := make([]string, 0, len(RequiredCNIPlugins))
	for _, plugin := range RequiredCNIPlugins {
		if !fileExists(CNIBinDir + "/" + plugin) {
			missing = append(missing, plugin)
		}
	}
	if len(missing) > 0 {
		script := fmt.Sprintf(
			"mkdir -p %s && curl -fsSL -o /tmp/cni-plugins.tgz "+
				"https://github.com/containernetworking/plugins/releases/download/%s/cni-plugins-linux-amd64-%s.tgz && "+
				"tar -C %s -xzf /tmp/cni-plugins.tgz && rm -f /tmp/cni-plugins.tgz",
			CNIBinDir, CNIPluginsVersion, CNIPluginsVersion, CNIBinDir)
		if _, err := i.Runner.Run(ctx, "bash", "-c", script); err != nil {
			return fmt.Errorf(
				"CNI plugins %v could not be installed: %w. Download the containernetworking plugins release manually into %s. The conveyor rails are missing",
				missing, err, CNIBinDir)
		}
		printer.OK("CNI plugins %s installed into %s.", CNIPluginsVersion, CNIBinDir)
	} else {
		printer.OK("CNI plugins are already installed in %s.", CNIBinDir)
	}

	if err := i.Runner.WriteFile(CNIConfigPath, CNIConfig(podCIDR), 0o644); err != nil {
		return err
	}
	printer.OK("Minimal bridge network configuration written to %s.", CNIConfigPath)

	if _, err := i.Runner.Run(ctx, "systemctl", "restart", "kubelet"); err != nil {
		printer.Warn("kubelet could not be restarted: %v. The node may stay NotReady until it is restarted manually.", err)
	}
	return nil
}

// installController installs the CRDs and the controller with Helm.
func (i *Installer) installController(ctx context.Context) error {
	printer := i.Printer

	chartPath := i.Options.ChartPath
	if chartPath == "" {
		chartPath = defaultChartPath()
	}
	if chartPath == "" {
		printer.Warn("Helm chart directory could not be located. Install the controller with: helm upgrade --install %s charts/brunernetes -n %s --create-namespace",
			HelmReleaseName, HelmNamespace)
		return nil
	}
	if !binaryAvailable("helm") && !i.Options.DryRun {
		printer.Warn("helm is not installed, so the controller was not applied. Install helm and rerun the installer. The paperwork office is closed.")
		return nil
	}

	if _, err := i.Runner.Run(ctx, "bash", "-c", fmt.Sprintf(
		"KUBECONFIG=%s helm upgrade --install %s %s --namespace %s --create-namespace --wait --timeout 5m",
		AdminKubeconfigPath, HelmReleaseName, chartPath, HelmNamespace)); err != nil {
		return fmt.Errorf(
			"Helm release %q could not be installed: %w. Check 'kubectl get pods -n %s'. The factory office could not be staffed",
			HelmReleaseName, err, HelmNamespace)
	}
	printer.OK("Brunernetes CRDs and controller are installed in namespace %s.", HelmNamespace)
	return nil
}

// applyFactoryResources creates the Togliatti and AvtoVAZ resources that match
// the selected preset.
func (i *Installer) applyFactoryResources(ctx context.Context) error {
	manifest := FactoryManifest(i.Options.Preset, KubernetesVersion)
	if _, err := i.Runner.RunWithInput(ctx, manifest, "bash", "-c",
		fmt.Sprintf("KUBECONFIG=%s kubectl apply -f -", AdminKubeconfigPath)); err != nil {
		i.Printer.Warn("Factory resources could not be applied: %v. Apply config/samples manually once the API server is reachable.", err)
		return nil
	}
	i.Printer.OK("Factory resources for preset %q are applied.", i.Options.Preset)
	return nil
}

// KubeadmConfig renders the kubeadm configuration for a single node.
func KubeadmConfig(version string, podCIDR string, advertiseAddress string) string {
	advertise := ""
	if advertiseAddress != "" {
		advertise = fmt.Sprintf("localAPIEndpoint:\n  advertiseAddress: %s\n  bindPort: 6443\n", advertiseAddress)
	}
	return fmt.Sprintf(`# Generated by brunerctl. Do not edit by hand: rerunning the installer
# overwrites this file.
apiVersion: kubeadm.k8s.io/v1beta4
kind: InitConfiguration
%snodeRegistration:
  criSocket: `+ContainerdSocketURL+`
  kubeletExtraArgs:
    - name: node-labels
      value: bruner.dev/factory=togliatti
---
apiVersion: kubeadm.k8s.io/v1beta4
kind: ClusterConfiguration
kubernetesVersion: %s
clusterName: brunernetes
networking:
  podSubnet: %s
  serviceSubnet: %s
  dnsDomain: %s
---
apiVersion: kubelet.config.k8s.io/v1beta1
kind: KubeletConfiguration
cgroupDriver: systemd
failSwapOn: true
---
apiVersion: kubeproxy.config.k8s.io/v1alpha1
kind: KubeProxyConfiguration
mode: iptables
`, advertise, version, podCIDR, ServiceCIDR, ClusterDNSDomain)
}

// CNIConfig renders the minimal bridge based CNI configuration. portmap is
// required for the loopback host port that publishes the application.
func CNIConfig(podCIDR string) string {
	return fmt.Sprintf(`{
  "cniVersion": "1.0.0",
  "name": "brunernetes",
  "plugins": [
    {
      "type": "bridge",
      "bridge": "bruner0",
      "isGateway": true,
      "ipMasq": true,
      "hairpinMode": true,
      "ipam": {
        "type": "host-local",
        "ranges": [[{ "subnet": "%s" }]],
        "routes": [{ "dst": "0.0.0.0/0" }]
      }
    },
    {
      "type": "portmap",
      "capabilities": { "portMappings": true }
    },
    {
      "type": "loopback"
    }
  ]
}
`, podCIDR)
}

// FactoryManifest renders the Togliatti and AvtoVAZ resources for a preset.
func FactoryManifest(preset string, version string) string {
	mode := PresetMode(preset)
	requireProbe := "false"
	requireRequests := "false"
	if mode.Profile == brunerv1alpha1.ProfileStrict {
		requireProbe = "true"
		requireRequests = "true"
	}
	return fmt.Sprintf(`apiVersion: bruner.dev/v1alpha1
kind: Togliatti
metadata:
  name: togliatti
spec:
  probeIntervalSeconds: 60
  expectedKubernetesVersion: %s
  expectedRuntime: containerd
---
apiVersion: bruner.dev/v1alpha1
kind: AvtoVAZ
metadata:
  name: avtovaz
spec:
  factoryRef: togliatti
  defaultMode:
    profile: %s
    rollout: %s
    qualityGate: %s
    shift: %s
  policy:
    requireReadinessProbe: %s
    requireResourceRequests: %s
    latestTagPolicy: Warn
    maxApplications: 1
    allowPublicExposure: false
`, version, mode.Profile, mode.Rollout, mode.QualityGate, mode.Shift, requireProbe, requireRequests)
}

// PresetMode maps an installation preset to a factory mode.
func PresetMode(preset string) brunerv1alpha1.BrunerMode {
	mode := brunerv1alpha1.DefaultMode()
	switch brunerv1alpha1.Profile(preset) {
	case brunerv1alpha1.ProfileShowroom:
		mode.Profile = brunerv1alpha1.ProfileShowroom
	case brunerv1alpha1.ProfileStrict:
		mode.Profile = brunerv1alpha1.ProfileStrict
		mode.QualityGate = brunerv1alpha1.QualityGateStrict
	case brunerv1alpha1.ProfileFriday:
		mode.Profile = brunerv1alpha1.ProfileFriday
	default:
		mode.Profile = brunerv1alpha1.ProfileGarage
	}
	return mode
}

// ValidPresets lists the accepted installation presets.
func ValidPresets() []string {
	return []string{
		string(brunerv1alpha1.ProfileGarage),
		string(brunerv1alpha1.ProfileShowroom),
		string(brunerv1alpha1.ProfileStrict),
		string(brunerv1alpha1.ProfileFriday),
	}
}

// installMode renders the execution mode for the summary block.
func installMode(dryRun bool) string {
	if dryRun {
		return "dry run, no host change is applied"
	}
	return "apply changes to this host"
}

// replacePauseImage rewrites the sandbox image in a containerd configuration.
func replacePauseImage(config string, image string) string {
	lines := strings.Split(config, "\n")
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "sandbox_image =") || strings.HasPrefix(trimmed, "sandbox =") {
			key, _, _ := strings.Cut(trimmed, "=")
			indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
			lines[index] = fmt.Sprintf("%s%s= %q", indent, key, image)
		}
	}
	return strings.Join(lines, "\n")
}

// sortedSysctlKeys returns the sysctl keys in a stable order so the generated
// file does not change between runs.
func sortedSysctlKeys() []string {
	keys := make([]string, 0, len(RequiredSysctls))
	for key := range RequiredSysctls {
		keys = append(keys, key)
	}
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[j] < keys[i] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	return keys
}

// defaultChartPath locates the bundled Helm chart relative to the working
// directory or the executable.
func defaultChartPath() string {
	candidates := []string{"charts/brunernetes", "/usr/local/share/brunernetes/charts/brunernetes"}
	if executable, err := os.Executable(); err == nil {
		candidates = append(candidates, parentDir(parentDir(executable))+"/charts/brunernetes")
	}
	for _, candidate := range candidates {
		if fileExists(candidate + "/Chart.yaml") {
			return candidate
		}
	}
	return ""
}
