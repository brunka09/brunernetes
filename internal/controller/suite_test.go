package controller

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	brunerv1alpha1 "github.com/brunka09/brunernetes/api/v1alpha1"
)

// startEnvironment boots a real API server and etcd through envtest.
//
// The control-plane binaries are not part of the module, so the suite skips
// itself when they are absent. Install them with:
//
//	make envtest
//
// or set KUBEBUILDER_ASSETS to a directory containing kube-apiserver, etcd and
// kubectl. CI installs them with setup-envtest, so the suite runs there.
func startEnvironment(t *testing.T) (*rest.Config, func()) {
	t.Helper()

	assets := os.Getenv("KUBEBUILDER_ASSETS")
	if assets == "" {
		t.Skip("KUBEBUILDER_ASSETS is not set, so no API server binaries are available. Run 'make envtest' or set KUBEBUILDER_ASSETS to run this suite.")
	}
	if _, err := os.Stat(filepath.Join(assets, "kube-apiserver")); err != nil {
		t.Skipf("kube-apiserver is missing from %s, so the envtest suite cannot run: %v", assets, err)
	}

	environment := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
		BinaryAssetsDirectory: assets,
	}

	// The API server picks its advertise address from the default route. A
	// container without one refuses to start, so the loopback address is set
	// explicitly. It is correct everywhere: the test control plane is only ever
	// reached from this process.
	environment.ControlPlane.GetAPIServer().Configure().Set("advertise-address", "127.0.0.1")

	config, err := environment.Start()
	if err != nil {
		t.Fatalf("the test control plane could not be started: %v", err)
	}
	return config, func() {
		if err := environment.Stop(); err != nil {
			t.Logf("the test control plane could not be stopped cleanly: %v", err)
		}
	}
}

// startManager runs the BrunerApp controller against the test control plane.
func startManager(t *testing.T, config *rest.Config) client.Client {
	t.Helper()

	managerScheme := testScheme(t)
	manager, err := ctrl.NewManager(config, ctrl.Options{Scheme: managerScheme})
	if err != nil {
		t.Fatalf("the manager could not be created: %v", err)
	}
	reconciler := &BrunerAppReconciler{
		Client:   manager.GetClient(),
		Scheme:   managerScheme,
		Recorder: manager.GetEventRecorderFor("brunernetes"),
	}
	if err := reconciler.SetupWithManager(manager); err != nil {
		t.Fatalf("the controller could not be registered: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() {
		if err := manager.Start(ctx); err != nil && ctx.Err() == nil {
			t.Errorf("the manager stopped unexpectedly: %v", err)
		}
	}()
	if !manager.GetCache().WaitForCacheSync(ctx) {
		t.Fatal("the controller cache did not sync")
	}

	directClient, err := client.New(config, client.Options{Scheme: managerScheme})
	if err != nil {
		t.Fatalf("the test client could not be created: %v", err)
	}
	return directClient
}

func TestControllerAgainstARealAPIServer(t *testing.T) {
	config, stop := startEnvironment(t)
	defer stop()

	if err := scheme.AddToScheme(testScheme(t)); err != nil {
		t.Fatalf("core types could not be registered: %v", err)
	}
	kubeClient := startManager(t, config)
	ctx := context.Background()

	namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "brunernetes-demo"}}
	if err := kubeClient.Create(ctx, namespace); err != nil {
		t.Fatalf("the namespace could not be created: %v", err)
	}

	app := &brunerv1alpha1.BrunerApp{
		ObjectMeta: metav1.ObjectMeta{Name: "showroom", Namespace: "brunernetes-demo"},
		Spec:       brunerv1alpha1.BrunerAppSpec{Image: "ghcr.io/brunka09/factory-showroom:v0.1.0"},
	}
	if err := kubeClient.Create(ctx, app); err != nil {
		t.Fatalf("the application could not be created: %v", err)
	}

	key := types.NamespacedName{Name: "showroom", Namespace: "brunernetes-demo"}

	// The controller must create the Deployment and the Service.
	deployment := &appsv1.Deployment{}
	eventually(t, 30*time.Second, func() bool {
		return kubeClient.Get(ctx, key, deployment) == nil
	}, "the Deployment was never created")

	service := &corev1.Service{}
	eventually(t, 30*time.Second, func() bool {
		return kubeClient.Get(ctx, key, service) == nil
	}, "the Service was never created")
	if service.Spec.Type != corev1.ServiceTypeClusterIP {
		t.Errorf("the Service must be a ClusterIP Service: got %q", service.Spec.Type)
	}

	// Status must be filled in. There is no kubelet in envtest, so the
	// application never becomes available; the conditions must still be there.
	stored := &brunerv1alpha1.BrunerApp{}
	eventually(t, 30*time.Second, func() bool {
		if err := kubeClient.Get(ctx, key, stored); err != nil {
			return false
		}
		return meta.FindStatusCondition(stored.Status.Conditions, brunerv1alpha1.ConditionAvailable) != nil
	}, "the Available condition was never reported")

	if stored.Status.DeploymentName != "showroom" {
		t.Errorf("status must name the Deployment: got %q", stored.Status.DeploymentName)
	}
	if stored.Status.LocalEndpoint != "http://127.0.0.1:30080" {
		t.Errorf("status must publish the loopback endpoint: got %q", stored.Status.LocalEndpoint)
	}

	// An image change must reach the Deployment.
	stored.Spec.Image = "ghcr.io/brunka09/factory-showroom:v0.2.0"
	if err := kubeClient.Update(ctx, stored); err != nil {
		t.Fatalf("the application could not be updated: %v", err)
	}
	eventually(t, 30*time.Second, func() bool {
		if err := kubeClient.Get(ctx, key, deployment); err != nil {
			return false
		}
		return deployment.Spec.Template.Spec.Containers[0].Image == "ghcr.io/brunka09/factory-showroom:v0.2.0"
	}, "the image change never reached the Deployment")

	// Deleting the application must take its objects with it through the owner
	// references. Garbage collection does not run in envtest, so the owner
	// reference itself is what the test asserts.
	if len(deployment.OwnerReferences) != 1 || deployment.OwnerReferences[0].UID != stored.UID {
		t.Errorf("the Deployment must be owned by the application: %+v", deployment.OwnerReferences)
	}
}

func TestCRDsAreInstallable(t *testing.T) {
	config, stop := startEnvironment(t)
	defer stop()

	kubeClient, err := client.New(config, client.Options{Scheme: testScheme(t)})
	if err != nil {
		t.Fatalf("the test client could not be created: %v", err)
	}
	ctx := context.Background()

	// Cluster-scoped kinds.
	togliatti := &brunerv1alpha1.Togliatti{
		ObjectMeta: metav1.ObjectMeta{Name: "togliatti"},
		Spec:       brunerv1alpha1.TogliattiSpec{CapacityProfile: brunerv1alpha1.CapacityCompact},
	}
	if err := kubeClient.Create(ctx, togliatti); err != nil {
		t.Fatalf("the Togliatti CRD does not accept a valid resource: %v", err)
	}

	avtovaz := &brunerv1alpha1.AvtoVAZ{
		ObjectMeta: metav1.ObjectMeta{Name: "avtovaz"},
		Spec: brunerv1alpha1.AvtoVAZSpec{
			DefaultMode: &brunerv1alpha1.BrunerMode{Profile: brunerv1alpha1.ProfileGarage},
		},
	}
	if err := kubeClient.Create(ctx, avtovaz); err != nil {
		t.Fatalf("the AvtoVAZ CRD does not accept a valid resource: %v", err)
	}

	// An unknown profile must be rejected by the generated schema, which is
	// what replaces an admission webhook in this design.
	invalid := &brunerv1alpha1.AvtoVAZ{
		ObjectMeta: metav1.ObjectMeta{Name: "invalid"},
		Spec: brunerv1alpha1.AvtoVAZSpec{
			DefaultMode: &brunerv1alpha1.BrunerMode{Profile: "monday"},
		},
	}
	if err := kubeClient.Create(ctx, invalid); err == nil {
		t.Error("the CRD schema must reject an unknown profile")
	}
}

// eventually polls a condition until it holds or the timeout expires.
func eventually(t *testing.T, timeout time.Duration, condition func() bool, message string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal(message)
}
