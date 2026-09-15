package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"

	brunerv1alpha1 "github.com/brunka09/brunernetes/api/v1alpha1"
)

// Version is stamped at build time with -ldflags.
var Version = "dev"

// globalOptions carries the flags shared by every command.
type globalOptions struct {
	kubeconfig string
	namespace  string
	printer    *Printer
}

// NewRootCommand builds the brunerctl command tree.
func NewRootCommand() *cobra.Command {
	options := &globalOptions{printer: NewPrinter()}

	root := &cobra.Command{
		Use:   "brunerctl",
		Short: "Operate a single-node Brunernetes factory",
		Long: strings.TrimSpace(`
brunerctl operates a single-node Brunernetes factory.

It is not a kubectl replacement. It installs the environment, reports the state
of the factory, deploys and inspects the single application, runs deliveries,
prints events and diagnoses faults.

Every message states the real technical condition first and adds factory wording
second. All output is English only.`),
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       Version,
	}

	root.PersistentFlags().StringVar(&options.kubeconfig, "kubeconfig", "",
		"Path to the kubeconfig file. Defaults to KUBECONFIG, then the user kubeconfig, then "+AdminKubeconfigPath+".")
	root.PersistentFlags().StringVarP(&options.namespace, "namespace", "n", "default",
		"Namespace used by the namespaced commands.")

	root.AddCommand(
		newInstallCommand(options),
		newUninstallCommand(options),
		newStatusCommand(options),
		newFactoryCommand(options),
		newAppCommand(options),
		newKonveerCommand(options),
		newEventsCommand(options),
		newDiagnoseCommand(options),
	)
	return root
}

// newInstallCommand builds "brunerctl install".
func newInstallCommand(options *globalOptions) *cobra.Command {
	install := InstallOptions{}

	command := &cobra.Command{
		Use:   "install",
		Short: "Install single-node Kubernetes and the Brunernetes controller",
		Long: strings.TrimSpace(`
Install a single-node Kubernetes environment with kubeadm and pinned upstream
components, then install the Brunernetes CRDs and controller with Helm.

The command changes the host and must run as root:

  sudo brunerctl install --preset garage

Use --dry-run first to see every command and file the installer would apply.`),
		RunE: func(command *cobra.Command, _ []string) error {
			if err := validatePreset(install.Preset); err != nil {
				return err
			}
			installer := &Installer{Printer: options.printer, Options: install}
			if install.DryRun {
				installer.Runner = &DryRunner{Printer: options.printer}
			} else {
				installer.Runner = &ExecRunner{Timeout: 20 * time.Minute}
			}
			return installer.Install(command.Context())
		},
	}

	command.Flags().StringVar(&install.Preset, "preset", "garage",
		"Installation preset: "+strings.Join(ValidPresets(), ", ")+".")
	command.Flags().BoolVar(&install.DryRun, "dry-run", false, "Print every host change instead of applying it.")
	command.Flags().BoolVar(&install.SkipPreflight, "skip-preflight", false, "Continue even when a blocking preflight check fails.")
	command.Flags().BoolVar(&install.SkipPackages, "skip-packages", false, "Assume the pinned packages and containerd are already installed.")
	command.Flags().BoolVar(&install.SkipCNI, "skip-cni", false, "Leave the CNI configuration untouched.")
	command.Flags().BoolVar(&install.SkipController, "skip-controller", false, "Do not install the Brunernetes Helm chart.")
	command.Flags().StringVar(&install.ChartPath, "chart", "", "Path to the Brunernetes Helm chart directory.")
	command.Flags().StringVar(&install.KubernetesVersion, "kubernetes-version", KubernetesVersion, "Kubernetes version to install.")
	command.Flags().StringVar(&install.PodCIDR, "pod-cidr", PodCIDR, "Pod network CIDR of the single node.")
	command.Flags().StringVar(&install.APIAdvertiseAddress, "api-advertise-address", "", "API server advertise address. Defaults to the kubeadm choice.")
	return command
}

// newUninstallCommand builds "brunerctl uninstall".
func newUninstallCommand(options *globalOptions) *cobra.Command {
	uninstall := UninstallOptions{}
	var confirm bool

	command := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove Brunernetes and the single-node control plane",
		Long: strings.TrimSpace(`
Remove the Brunernetes controller, reset the control plane with kubeadm and
delete the files the installer generated.

The command changes the host and must run as root:

  sudo brunerctl uninstall --yes

This is destructive: the cluster, its workloads and its etcd data are removed.`),
		RunE: func(command *cobra.Command, _ []string) error {
			if !confirm && !uninstall.DryRun {
				return fmt.Errorf(
					"uninstall removes the cluster, its workloads and its etcd data, so it requires confirmation. " +
						"Rerun with --yes, or with --dry-run to see what would happen. The line is not dismantled without a signature")
			}
			uninstaller := &Uninstaller{Printer: options.printer, Options: uninstall}
			if uninstall.DryRun {
				uninstaller.Runner = &DryRunner{Printer: options.printer}
			} else {
				uninstaller.Runner = &ExecRunner{Timeout: 10 * time.Minute}
			}
			return uninstaller.Uninstall(command.Context())
		},
	}

	command.Flags().BoolVar(&uninstall.DryRun, "dry-run", false, "Print every host change instead of applying it.")
	command.Flags().BoolVar(&uninstall.KeepPackages, "keep-packages", false, "Keep kubeadm, kubelet and kubectl installed.")
	command.Flags().BoolVar(&uninstall.Purge, "purge", false, "Also remove containerd and its images.")
	command.Flags().BoolVar(&confirm, "yes", false, "Confirm the destructive operation.")
	return command
}

// newStatusCommand builds "brunerctl status".
func newStatusCommand(options *globalOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the state of the factory and its application",
		RunE: func(command *cobra.Command, _ []string) error {
			return withClient(command.Context(), options, func(ctx context.Context, kubeClient client.Client) error {
				return Status(ctx, kubeClient, options.printer)
			})
		},
	}
}

// newFactoryCommand builds "brunerctl factory".
func newFactoryCommand(options *globalOptions) *cobra.Command {
	factoryCommand := &cobra.Command{
		Use:   "factory",
		Short: "Inspect the factory environment",
	}
	factoryCommand.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show the Togliatti factory environment in detail",
		RunE: func(command *cobra.Command, _ []string) error {
			return withClient(command.Context(), options, func(ctx context.Context, kubeClient client.Client) error {
				return FactoryStatus(ctx, kubeClient, options.printer)
			})
		},
	})
	return factoryCommand
}

// newAppCommand builds "brunerctl app".
func newAppCommand(options *globalOptions) *cobra.Command {
	appCommand := &cobra.Command{
		Use:     "app",
		Short:   "Deploy and inspect the application",
		Aliases: []string{"apps"},
	}

	var manifest string
	var wait time.Duration
	deploy := &cobra.Command{
		Use:   "deploy",
		Short: "Apply a BrunerApp manifest",
		RunE: func(command *cobra.Command, _ []string) error {
			if manifest == "" {
				return fmt.Errorf("no manifest was given. Use: brunerctl app deploy -f app.yaml. The line needs a work order")
			}
			app, err := DecodeBrunerApp(manifest)
			if err != nil {
				return err
			}
			if command.Flags().Changed("namespace") {
				app.Namespace = options.namespace
			}
			return withClient(command.Context(), options, func(ctx context.Context, kubeClient client.Client) error {
				return DeployApp(ctx, kubeClient, options.printer, app, wait)
			})
		},
	}
	deploy.Flags().StringVarP(&manifest, "filename", "f", "", "Path to a BrunerApp manifest, or - to read standard input.")
	deploy.Flags().DurationVar(&wait, "wait", 3*time.Minute, "How long to wait for the application to become available. Use 0 to return immediately.")

	list := &cobra.Command{
		Use:     "list",
		Short:   "List the applications of the factory",
		Aliases: []string{"ls"},
		RunE: func(command *cobra.Command, _ []string) error {
			namespace := ""
			if command.Flags().Changed("namespace") {
				namespace = options.namespace
			}
			return withClient(command.Context(), options, func(ctx context.Context, kubeClient client.Client) error {
				return ListApps(ctx, kubeClient, options.printer, namespace)
			})
		},
	}

	inspect := &cobra.Command{
		Use:   "inspect <name>",
		Short: "Show everything known about one application",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			return withClient(command.Context(), options, func(ctx context.Context, kubeClient client.Client) error {
				return InspectApp(ctx, kubeClient, options.printer, args[0], options.namespace)
			})
		},
	}

	appCommand.AddCommand(deploy, list, inspect)
	return appCommand
}

// newKonveerCommand builds "brunerctl konveer".
func newKonveerCommand(options *globalOptions) *cobra.Command {
	konveerCommand := &cobra.Command{
		Use:   "konveer",
		Short: "Run a delivery workflow for an existing image",
	}

	var manifest string
	apply := &cobra.Command{
		Use:   "apply",
		Short: "Apply a Konveer manifest and run the delivery",
		RunE: func(command *cobra.Command, _ []string) error {
			if manifest == "" {
				return fmt.Errorf("no manifest was given. Use: brunerctl konveer apply -f konveer.yaml. The line needs a delivery order")
			}
			konveer, err := DecodeKonveer(manifest)
			if err != nil {
				return err
			}
			if command.Flags().Changed("namespace") {
				konveer.Namespace = options.namespace
			}
			return withClient(command.Context(), options, func(ctx context.Context, kubeClient client.Client) error {
				return ApplyKonveer(ctx, kubeClient, options.printer, konveer)
			})
		},
	}
	apply.Flags().StringVarP(&manifest, "filename", "f", "", "Path to a Konveer manifest, or - to read standard input.")

	konveerCommand.AddCommand(apply)
	return konveerCommand
}

// newEventsCommand builds "brunerctl events".
func newEventsCommand(options *globalOptions) *cobra.Command {
	var limit int
	var allNamespaces bool

	command := &cobra.Command{
		Use:   "events",
		Short: "Show recent events of the Brunernetes resources",
		RunE: func(command *cobra.Command, _ []string) error {
			namespace := options.namespace
			if allNamespaces {
				namespace = ""
			}
			return withClient(command.Context(), options, func(ctx context.Context, kubeClient client.Client) error {
				return Events(ctx, kubeClient, options.printer, namespace, limit)
			})
		},
	}
	command.Flags().IntVar(&limit, "limit", 30, "Maximum number of events to print.")
	command.Flags().BoolVarP(&allNamespaces, "all-namespaces", "A", false, "Show events from every namespace.")
	return command
}

// newDiagnoseCommand builds "brunerctl diagnose".
func newDiagnoseCommand(options *globalOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "diagnose",
		Short: "Check the whole installation and report the first broken thing",
		RunE: func(command *cobra.Command, _ []string) error {
			return withClient(command.Context(), options, func(ctx context.Context, kubeClient client.Client) error {
				return Diagnose(ctx, kubeClient, options.printer)
			})
		},
	}
}

// withClient builds a Kubernetes client and hands it to the command body.
func withClient(ctx context.Context, options *globalOptions, body func(context.Context, client.Client) error) error {
	kubeClient, err := NewClient(options.kubeconfig)
	if err != nil {
		return err
	}
	return body(ctx, kubeClient)
}

// validatePreset checks the installation preset.
func validatePreset(preset string) error {
	for _, valid := range ValidPresets() {
		if preset == valid {
			return nil
		}
	}
	return fmt.Errorf(
		"preset %q is unknown. Allowed presets are %s. That shift plan does not exist in this plant",
		preset, strings.Join(ValidPresets(), ", "))
}

// ensure the API package is linked into the CLI binary even when a build tag
// removes an optional command.
var _ = brunerv1alpha1.GroupVersion
