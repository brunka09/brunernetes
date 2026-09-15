// Command manager runs the Brunernetes controller manager. It reconciles the
// single application of the factory and reports the state of the assembly line
// through Kubernetes conditions and events.
package main

import (
	"flag"
	"fmt"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	brunerv1alpha1 "github.com/brunka09/brunernetes/api/v1alpha1"
	"github.com/brunka09/brunernetes/internal/controller"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

// version is stamped at build time with -ldflags.
var version = "dev"

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(brunerv1alpha1.AddToScheme(scheme))
}

func main() {
	var metricsAddr string
	var probeAddr string
	var enableLeaderElection bool
	var showVersion bool

	flag.StringVar(&metricsAddr, "metrics-bind-address", "0", "Address the metric endpoint binds to. Use 0 to disable it.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "Address the health probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election. A single-node factory runs one manager, so this is off by default.")
	flag.BoolVar(&showVersion, "version", false, "Print the manager version and exit.")

	options := zap.Options{Development: false}
	options.BindFlags(flag.CommandLine)
	flag.Parse()

	if showVersion {
		fmt.Printf("brunernetes-manager %s\n", version)
		return
	}

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&options)))

	manager, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: metricsAddr},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "brunernetes.bruner.dev",
	})
	if err != nil {
		setupLog.Error(err, "Controller manager could not be created. Check the kubeconfig and the API server address. The factory office did not open.")
		os.Exit(1)
	}

	if err := (&controller.BrunerAppReconciler{
		Client:   manager.GetClient(),
		Scheme:   manager.GetScheme(),
		Recorder: manager.GetEventRecorderFor("brunerapp-controller"),
	}).SetupWithManager(manager); err != nil {
		setupLog.Error(err, "BrunerApp controller could not be registered. The assembly line has no supervisor.")
		os.Exit(1)
	}

	if err := manager.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "Health check could not be registered.")
		os.Exit(1)
	}
	if err := manager.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "Readiness check could not be registered.")
		os.Exit(1)
	}

	setupLog.Info("Starting the Brunernetes controller manager. The factory office is open.", "version", version)
	if err := manager.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "Controller manager stopped with an error. The shift ended unexpectedly.")
		os.Exit(1)
	}
}
