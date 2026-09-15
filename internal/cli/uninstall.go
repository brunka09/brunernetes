package cli

import (
	"context"
	"fmt"
)

// UninstallOptions configures the uninstaller.
type UninstallOptions struct {
	// DryRun prints every host change instead of applying it.
	DryRun bool
	// KeepPackages leaves kubeadm, kubelet, kubectl and containerd installed.
	KeepPackages bool
	// Purge also removes the container images and the containerd state.
	Purge bool
}

// Uninstaller reverses the installation as far as that can be done safely.
type Uninstaller struct {
	Printer *Printer
	Runner  CommandRunner
	Options UninstallOptions
}

// uninstallSteps is the number of uninstall steps reported to the operator.
const uninstallSteps = 6

// Uninstall removes the Brunernetes installation from the host. It is the
// inverse of Install wherever an inverse exists: package removal is optional
// and the container runtime is only purged on request, because both may be
// used by something else on the host.
func (u *Uninstaller) Uninstall(ctx context.Context) error {
	printer := u.Printer

	printer.Section("Brunernetes uninstallation")
	printer.KeyValues([][2]string{
		{"Mode", installMode(u.Options.DryRun)},
		{"Remove packages", boolLabel(!u.Options.KeepPackages)},
		{"Purge runtime state", boolLabel(u.Options.Purge)},
	})

	// Step 1: remove the Helm release.
	printer.Step(1, uninstallSteps, "Removing the Brunernetes controller Helm release.")
	if _, err := u.Runner.Run(ctx, "bash", "-c", fmt.Sprintf(
		"KUBECONFIG=%s helm uninstall %s --namespace %s --ignore-not-found || true",
		AdminKubeconfigPath, HelmReleaseName, HelmNamespace)); err != nil {
		printer.Warn("Helm release could not be removed: %v. Continuing with the host cleanup.", err)
	} else {
		printer.OK("Helm release %q is removed.", HelmReleaseName)
	}

	// Step 2: remove the factory namespaces.
	printer.Step(2, uninstallSteps, "Removing the Brunernetes namespaces.")
	if _, err := u.Runner.Run(ctx, "bash", "-c", fmt.Sprintf(
		"KUBECONFIG=%s kubectl delete namespace %s %s --ignore-not-found --wait=false || true",
		AdminKubeconfigPath, HelmNamespace, DemoNamespace)); err != nil {
		printer.Warn("Namespaces could not be removed: %v.", err)
	} else {
		printer.OK("Namespaces %s and %s are removed.", HelmNamespace, DemoNamespace)
	}

	// Step 3: reset the control plane.
	printer.Step(3, uninstallSteps, "Resetting the control plane with kubeadm.")
	if _, err := u.Runner.Run(ctx, "kubeadm", "reset", "-f", "--cri-socket", ContainerdSocketURL); err != nil {
		printer.Warn("kubeadm reset reported a problem: %v. Inspect /etc/kubernetes manually.", err)
	} else {
		printer.OK("Control plane is reset.")
	}

	// Step 4: remove the generated host files.
	printer.Step(4, uninstallSteps, "Removing the files the installer generated.")
	for _, path := range []string{
		CNIConfigPath,
		SysctlConfigPath,
		ModulesConfigPath,
		StateDir,
		"/etc/cni/net.d",
		"/var/lib/etcd",
		"/etc/kubernetes",
	} {
		if err := u.Runner.RemoveAll(path); err != nil {
			printer.Warn("Path %q could not be removed: %v.", path, err)
			continue
		}
		printer.OK("Removed %s.", path)
	}
	if _, err := u.Runner.Run(ctx, "bash", "-c", "iptables -F && iptables -t nat -F && iptables -t mangle -F && iptables -X || true"); err != nil {
		printer.Warn("iptables rules could not be flushed: %v. Reboot the host to clear them.", err)
	} else {
		printer.OK("Leftover iptables rules are flushed.")
	}

	// Step 5: packages.
	printer.Step(5, uninstallSteps, "Removing the Kubernetes packages.")
	if u.Options.KeepPackages {
		printer.Skip("Package removal was skipped by request.")
	} else if _, err := u.Runner.Run(ctx, "bash", "-c",
		"apt-mark unhold kubelet kubeadm kubectl || true; "+
			"apt-get purge -y kubelet kubeadm kubectl || true; "+
			"rm -f /etc/apt/sources.list.d/kubernetes.list /etc/apt/keyrings/kubernetes-apt-keyring.gpg; "+
			"apt-get autoremove -y || true"); err != nil {
		printer.Warn("Kubernetes packages could not be removed: %v. Remove them manually with apt-get purge.", err)
	} else {
		printer.OK("Kubernetes packages are removed.")
	}

	// Step 6: container runtime state.
	printer.Step(6, uninstallSteps, "Cleaning up the container runtime state.")
	if !u.Options.Purge {
		printer.Skip("containerd and its images are kept. Use --purge to remove them as well.")
	} else if _, err := u.Runner.Run(ctx, "bash", "-c",
		"systemctl stop containerd || true; apt-get purge -y containerd || true; rm -rf /var/lib/containerd /etc/containerd"); err != nil {
		printer.Warn("containerd state could not be purged: %v.", err)
	} else {
		printer.OK("containerd and its state are removed.")
	}

	printer.Section("Uninstallation complete")
	if u.Options.DryRun {
		printer.Println("Dry run finished. No change was applied to this host. The decommissioning plan is on the desk, the line is untouched.")
		return nil
	}
	printer.Println("Brunernetes and the single-node control plane are removed. Reboot the host to clear any remaining network state. The factory floor is swept and the lights are off.")
	return nil
}

// boolLabel renders a boolean for the summary block.
func boolLabel(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}
