package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	brunerv1alpha1 "github.com/brunka09/brunernetes/api/v1alpha1"
)

// writeManifest stores a manifest in a temporary directory.
func writeManifest(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "manifest.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("the manifest could not be written: %v", err)
	}
	return path
}

// fakeCluster returns a client preloaded with the given objects.
func fakeCluster(t *testing.T, objects ...client.Object) client.Client {
	t.Helper()
	testScheme := Scheme()
	if err := scheme.AddToScheme(testScheme); err != nil {
		t.Fatalf("core types could not be registered: %v", err)
	}
	return fake.NewClientBuilder().
		WithScheme(testScheme).
		WithObjects(objects...).
		WithStatusSubresource(&brunerv1alpha1.BrunerApp{}, &brunerv1alpha1.Konveer{}).
		Build()
}

func TestDecodeBrunerApp(t *testing.T) {
	path := writeManifest(t, `
apiVersion: bruner.dev/v1alpha1
kind: BrunerApp
metadata:
  name: showroom
  namespace: brunernetes-demo
spec:
  image: ghcr.io/brunka09/factory-showroom:v0.1.0
  port: 8080
  healthPath: /healthz
`)
	app, err := DecodeBrunerApp(path)
	if err != nil {
		t.Fatalf("a valid manifest must decode: %v", err)
	}
	if app.Name != "showroom" || app.Namespace != "brunernetes-demo" {
		t.Errorf("unexpected object meta: %s/%s", app.Namespace, app.Name)
	}
	if app.Spec.Image != "ghcr.io/brunka09/factory-showroom:v0.1.0" {
		t.Errorf("image: got %q", app.Spec.Image)
	}
	if app.Spec.Port != 8080 {
		t.Errorf("port: got %d", app.Spec.Port)
	}
}

func TestDecodeBrunerAppDefaultsTheNamespace(t *testing.T) {
	path := writeManifest(t, `
apiVersion: bruner.dev/v1alpha1
kind: BrunerApp
metadata:
  name: showroom
spec:
  image: nginx:1.27-alpine
`)
	app, err := DecodeBrunerApp(path)
	if err != nil {
		t.Fatalf("a valid manifest must decode: %v", err)
	}
	if app.Namespace != "default" {
		t.Errorf("namespace: got %q, want %q", app.Namespace, "default")
	}
}

func TestDecodeBrunerAppRejectsBadManifests(t *testing.T) {
	cases := map[string]string{
		"a Deployment is not a BrunerApp": `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: showroom
`,
		"a manifest without a name": `
apiVersion: bruner.dev/v1alpha1
kind: BrunerApp
spec:
  image: nginx:1.27
`,
		"a manifest without an image": `
apiVersion: bruner.dev/v1alpha1
kind: BrunerApp
metadata:
  name: showroom
spec: {}
`,
		"an empty manifest": "",
		"a broken document": `
apiVersion: bruner.dev/v1alpha1
kind: BrunerApp
metadata:
	name: [unclosed
`,
	}

	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeBrunerApp(writeManifest(t, content)); err == nil {
				t.Fatal("the manifest was expected to be rejected")
			}
		})
	}

	if _, err := DecodeBrunerApp(filepath.Join(t.TempDir(), "missing.yaml")); err == nil {
		t.Error("a missing file must be reported")
	}
}

func TestDecodeKonveer(t *testing.T) {
	path := writeManifest(t, `
apiVersion: bruner.dev/v1alpha1
kind: Konveer
metadata:
  name: showroom-v1
  namespace: brunernetes-demo
spec:
  appRef: showroom
  image: ghcr.io/brunka09/factory-showroom:v0.1.0
`)
	konveer, err := DecodeKonveer(path)
	if err != nil {
		t.Fatalf("a valid manifest must decode: %v", err)
	}
	if konveer.Spec.AppRef != "showroom" {
		t.Errorf("appRef: got %q", konveer.Spec.AppRef)
	}

	missingRef := writeManifest(t, `
apiVersion: bruner.dev/v1alpha1
kind: Konveer
metadata:
  name: showroom-v1
spec:
  image: nginx:1.27
`)
	if _, err := DecodeKonveer(missingRef); err == nil {
		t.Error("a delivery without a target application must be rejected")
	}
}

func TestDeployAppCreatesThenUpdates(t *testing.T) {
	kubeClient := fakeCluster(t)
	printer := quietPrinter()
	ctx := context.Background()

	app := &brunerv1alpha1.BrunerApp{
		ObjectMeta: metav1.ObjectMeta{Name: "showroom", Namespace: "brunernetes-demo"},
		Spec:       brunerv1alpha1.BrunerAppSpec{Image: "ghcr.io/brunka09/showroom:v0.1.0"},
	}
	if err := DeployApp(ctx, kubeClient, printer, app, 0); err != nil {
		t.Fatalf("the application could not be created: %v", err)
	}

	updated := &brunerv1alpha1.BrunerApp{
		ObjectMeta: metav1.ObjectMeta{Name: "showroom", Namespace: "brunernetes-demo"},
		Spec:       brunerv1alpha1.BrunerAppSpec{Image: "ghcr.io/brunka09/showroom:v0.2.0"},
	}
	if err := DeployApp(ctx, kubeClient, printer, updated, 0); err != nil {
		t.Fatalf("the application could not be updated: %v", err)
	}

	stored := &brunerv1alpha1.BrunerApp{}
	key := types.NamespacedName{Name: "showroom", Namespace: "brunernetes-demo"}
	if err := kubeClient.Get(ctx, key, stored); err != nil {
		t.Fatalf("the application could not be read: %v", err)
	}
	if stored.Spec.Image != "ghcr.io/brunka09/showroom:v0.2.0" {
		t.Errorf("the second apply must update the image: got %q", stored.Spec.Image)
	}

	apps := &brunerv1alpha1.BrunerAppList{}
	if err := kubeClient.List(ctx, apps); err != nil {
		t.Fatalf("the applications could not be listed: %v", err)
	}
	if len(apps.Items) != 1 {
		t.Errorf("applying twice must not create a second application: got %d", len(apps.Items))
	}
}

func TestListAppsReportsAnEmptyFactory(t *testing.T) {
	builder := &strings.Builder{}
	printer := &Printer{Out: builder, Err: builder}
	if err := ListApps(context.Background(), fakeCluster(t), printer, ""); err != nil {
		t.Fatalf("listing an empty factory must not fail: %v", err)
	}
	if !strings.Contains(builder.String(), "No application is deployed") {
		t.Errorf("an empty factory must be stated plainly: %q", builder.String())
	}
}

func TestInspectAppReportsAMissingApplication(t *testing.T) {
	err := InspectApp(context.Background(), fakeCluster(t), quietPrinter(), "ghost", "default")
	if err == nil {
		t.Fatal("inspecting a missing application must fail")
	}
	if !strings.Contains(err.Error(), "brunerctl app list") {
		t.Errorf("the error must point at the next command: %q", err.Error())
	}
}
