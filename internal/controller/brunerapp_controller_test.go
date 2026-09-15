package controller

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	brunerv1alpha1 "github.com/brunka09/brunernetes/api/v1alpha1"
	"github.com/brunka09/brunernetes/internal/factory"
)

// testScheme returns a scheme with the core types and the Brunernetes types.
func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	testScheme := runtime.NewScheme()
	if err := scheme.AddToScheme(testScheme); err != nil {
		t.Fatalf("core types could not be registered: %v", err)
	}
	if err := brunerv1alpha1.AddToScheme(testScheme); err != nil {
		t.Fatalf("Brunernetes types could not be registered: %v", err)
	}
	return testScheme
}

// newReconciler builds a reconciler backed by a fake client.
func newReconciler(t *testing.T, objects ...client.Object) (*BrunerAppReconciler, client.Client) {
	t.Helper()
	testScheme := testScheme(t)
	fakeClient := fake.NewClientBuilder().
		WithScheme(testScheme).
		WithObjects(objects...).
		WithStatusSubresource(&brunerv1alpha1.BrunerApp{}, &brunerv1alpha1.Konveer{}).
		Build()
	return &BrunerAppReconciler{
		Client:   fakeClient,
		Scheme:   testScheme,
		Recorder: record.NewFakeRecorder(64),
	}, fakeClient
}

// sampleApp returns an application used by the reconciliation tests.
func sampleApp(mutators ...func(*brunerv1alpha1.BrunerApp)) *brunerv1alpha1.BrunerApp {
	app := &brunerv1alpha1.BrunerApp{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "showroom",
			Namespace:  "brunernetes-demo",
			Generation: 1,
		},
		Spec: brunerv1alpha1.BrunerAppSpec{Image: "ghcr.io/brunka09/showroom:v0.1.0"},
	}
	for _, mutate := range mutators {
		mutate(app)
	}
	return app
}

// request addresses the sample application.
func request(app *brunerv1alpha1.BrunerApp) ctrl.Request {
	return ctrl.Request{NamespacedName: types.NamespacedName{Name: app.Name, Namespace: app.Namespace}}
}

func TestReconcileCreatesTheOwnedObjects(t *testing.T) {
	app := sampleApp()
	reconciler, fakeClient := newReconciler(t, app)
	ctx := context.Background()

	if _, err := reconciler.Reconcile(ctx, request(app)); err != nil {
		t.Fatalf("first reconciliation failed: %v", err)
	}

	deployment := &appsv1.Deployment{}
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: "showroom", Namespace: "brunernetes-demo"}, deployment); err != nil {
		t.Fatalf("the Deployment was not created: %v", err)
	}
	if got := deployment.Spec.Template.Spec.Containers[0].Image; got != app.Spec.Image {
		t.Errorf("deployed image: got %q, want %q", got, app.Spec.Image)
	}
	if len(deployment.OwnerReferences) != 1 || deployment.OwnerReferences[0].Name != app.Name {
		t.Errorf("the Deployment must be owned by the BrunerApp: %+v", deployment.OwnerReferences)
	}

	service := &corev1.Service{}
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: "showroom", Namespace: "brunernetes-demo"}, service); err != nil {
		t.Fatalf("the Service was not created: %v", err)
	}
	if service.Spec.Type != corev1.ServiceTypeClusterIP {
		t.Errorf("the Service must be a ClusterIP Service: got %q", service.Spec.Type)
	}
	if len(service.OwnerReferences) != 1 {
		t.Errorf("the Service must be owned by the BrunerApp: %+v", service.OwnerReferences)
	}

	reconciled := &brunerv1alpha1.BrunerApp{}
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: "showroom", Namespace: "brunernetes-demo"}, reconciled); err != nil {
		t.Fatalf("the application could not be read back: %v", err)
	}
	if reconciled.Status.ObservedGeneration != app.Generation {
		t.Errorf("observed generation: got %d, want %d", reconciled.Status.ObservedGeneration, app.Generation)
	}
	if reconciled.Status.DeploymentName != "showroom" || reconciled.Status.ServiceName != "showroom" {
		t.Errorf("status must name the owned objects: %+v", reconciled.Status)
	}
	if reconciled.Status.LocalEndpoint != "http://127.0.0.1:30080" {
		t.Errorf("status must publish the loopback endpoint: got %q", reconciled.Status.LocalEndpoint)
	}
	if reconciled.Status.AppliedImage != app.Spec.Image {
		t.Errorf("status must record the applied image: got %q", reconciled.Status.AppliedImage)
	}
	if meta.FindStatusCondition(reconciled.Status.Conditions, brunerv1alpha1.ConditionAvailable) == nil {
		t.Error("the Available condition must always be reported")
	}
}

func TestReconcileIsIdempotent(t *testing.T) {
	app := sampleApp()
	reconciler, fakeClient := newReconciler(t, app)
	ctx := context.Background()

	if _, err := reconciler.Reconcile(ctx, request(app)); err != nil {
		t.Fatalf("first reconciliation failed: %v", err)
	}
	first := &appsv1.Deployment{}
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: "showroom", Namespace: "brunernetes-demo"}, first); err != nil {
		t.Fatalf("the Deployment was not created: %v", err)
	}

	if _, err := reconciler.Reconcile(ctx, request(app)); err != nil {
		t.Fatalf("second reconciliation failed: %v", err)
	}
	second := &appsv1.Deployment{}
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: "showroom", Namespace: "brunernetes-demo"}, second); err != nil {
		t.Fatalf("the Deployment disappeared: %v", err)
	}

	if first.ResourceVersion != second.ResourceVersion {
		t.Errorf("a second reconciliation must not rewrite the Deployment: %s then %s",
			first.ResourceVersion, second.ResourceVersion)
	}
}

func TestReconcileAppliesAnImageChange(t *testing.T) {
	app := sampleApp()
	reconciler, fakeClient := newReconciler(t, app)
	ctx := context.Background()

	if _, err := reconciler.Reconcile(ctx, request(app)); err != nil {
		t.Fatalf("first reconciliation failed: %v", err)
	}

	stored := &brunerv1alpha1.BrunerApp{}
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: "showroom", Namespace: "brunernetes-demo"}, stored); err != nil {
		t.Fatalf("the application could not be read: %v", err)
	}
	stored.Spec.Image = "ghcr.io/brunka09/showroom:v0.2.0"
	stored.Generation = 2
	if err := fakeClient.Update(ctx, stored); err != nil {
		t.Fatalf("the application could not be updated: %v", err)
	}

	if _, err := reconciler.Reconcile(ctx, request(app)); err != nil {
		t.Fatalf("reconciliation after the image change failed: %v", err)
	}

	deployment := &appsv1.Deployment{}
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: "showroom", Namespace: "brunernetes-demo"}, deployment); err != nil {
		t.Fatalf("the Deployment could not be read: %v", err)
	}
	if got := deployment.Spec.Template.Spec.Containers[0].Image; got != "ghcr.io/brunka09/showroom:v0.2.0" {
		t.Errorf("the new image must reach the Deployment: got %q", got)
	}
}

func TestReconcileBlocksAPolicyViolation(t *testing.T) {
	// The strict quality gate requires a readiness probe; this application has
	// none, so the rollout must be held back before any object is created.
	policy := &brunerv1alpha1.AvtoVAZ{
		ObjectMeta: metav1.ObjectMeta{Name: "avtovaz"},
		Spec: brunerv1alpha1.AvtoVAZSpec{
			DefaultMode: &brunerv1alpha1.BrunerMode{QualityGate: brunerv1alpha1.QualityGateStrict},
		},
	}
	app := sampleApp()
	reconciler, fakeClient := newReconciler(t, app, policy)
	ctx := context.Background()

	if _, err := reconciler.Reconcile(ctx, request(app)); err != nil {
		t.Fatalf("reconciliation returned an error instead of holding the rollout: %v", err)
	}

	deployment := &appsv1.Deployment{}
	err := fakeClient.Get(ctx, types.NamespacedName{Name: "showroom", Namespace: "brunernetes-demo"}, deployment)
	if !apierrors.IsNotFound(err) {
		t.Fatalf("a blocked application must not create a Deployment: %v", err)
	}

	stored := &brunerv1alpha1.BrunerApp{}
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: "showroom", Namespace: "brunernetes-demo"}, stored); err != nil {
		t.Fatalf("the application could not be read: %v", err)
	}
	enforced := meta.FindStatusCondition(stored.Status.Conditions, brunerv1alpha1.ConditionPolicyEnforced)
	if enforced == nil || enforced.Status != metav1.ConditionFalse {
		t.Fatalf("PolicyEnforced must report the violation: %+v", enforced)
	}
	if stored.Status.Stage != brunerv1alpha1.StageInspection {
		t.Errorf("a blocked application must be held at inspection: got %q", stored.Status.Stage)
	}
	degraded := meta.FindStatusCondition(stored.Status.Conditions, brunerv1alpha1.ConditionDegraded)
	if degraded == nil || degraded.Status != metav1.ConditionTrue {
		t.Errorf("a blocked application must be reported as degraded: %+v", degraded)
	}
}

func TestReconcileAcceptsAPolicyCompliantApplication(t *testing.T) {
	policy := &brunerv1alpha1.AvtoVAZ{
		ObjectMeta: metav1.ObjectMeta{Name: "avtovaz"},
		Spec: brunerv1alpha1.AvtoVAZSpec{
			DefaultMode: &brunerv1alpha1.BrunerMode{Profile: brunerv1alpha1.ProfileShowroom},
			Policy:      &brunerv1alpha1.FactoryPolicy{AllowedRegistries: []string{"ghcr.io"}},
		},
	}
	app := sampleApp()
	reconciler, fakeClient := newReconciler(t, app, policy)
	ctx := context.Background()

	if _, err := reconciler.Reconcile(ctx, request(app)); err != nil {
		t.Fatalf("reconciliation failed: %v", err)
	}

	deployment := &appsv1.Deployment{}
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: "showroom", Namespace: "brunernetes-demo"}, deployment); err != nil {
		t.Fatalf("a compliant application must be rolled out: %v", err)
	}
	if got := deployment.Labels[factory.LabelProfile]; got != string(brunerv1alpha1.ProfileShowroom) {
		t.Errorf("the factory default profile must reach the owned objects: got %q", got)
	}

	stored := &brunerv1alpha1.BrunerApp{}
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: "showroom", Namespace: "brunernetes-demo"}, stored); err != nil {
		t.Fatalf("the application could not be read: %v", err)
	}
	if stored.Status.EffectiveMode == nil || stored.Status.EffectiveMode.Profile != brunerv1alpha1.ProfileShowroom {
		t.Errorf("status must report the effective mode: %+v", stored.Status.EffectiveMode)
	}
}

func TestReconcileReportsAvailabilityFromTheDeployment(t *testing.T) {
	app := sampleApp()
	reconciler, fakeClient := newReconciler(t, app)
	ctx := context.Background()

	if _, err := reconciler.Reconcile(ctx, request(app)); err != nil {
		t.Fatalf("first reconciliation failed: %v", err)
	}

	deployment := &appsv1.Deployment{}
	key := types.NamespacedName{Name: "showroom", Namespace: "brunernetes-demo"}
	if err := fakeClient.Get(ctx, key, deployment); err != nil {
		t.Fatalf("the Deployment could not be read: %v", err)
	}
	deployment.Status = appsv1.DeploymentStatus{
		Replicas:          1,
		UpdatedReplicas:   1,
		ReadyReplicas:     1,
		AvailableReplicas: 1,
	}
	if err := fakeClient.Status().Update(ctx, deployment); err != nil {
		t.Fatalf("the Deployment status could not be written: %v", err)
	}

	if _, err := reconciler.Reconcile(ctx, request(app)); err != nil {
		t.Fatalf("second reconciliation failed: %v", err)
	}

	stored := &brunerv1alpha1.BrunerApp{}
	if err := fakeClient.Get(ctx, key, stored); err != nil {
		t.Fatalf("the application could not be read: %v", err)
	}
	available := meta.FindStatusCondition(stored.Status.Conditions, brunerv1alpha1.ConditionAvailable)
	if available == nil || available.Status != metav1.ConditionTrue {
		t.Fatalf("a fully ready Deployment must make the application available: %+v", available)
	}
	if stored.Status.Stage != brunerv1alpha1.StageShowroom {
		t.Errorf("a ready application must reach the showroom stage: got %q", stored.Status.Stage)
	}
	if stored.Status.ReadyReplicas != 1 {
		t.Errorf("ready replicas: got %d, want 1", stored.Status.ReadyReplicas)
	}
}

func TestReconcileIgnoresAMissingApplication(t *testing.T) {
	reconciler, _ := newReconciler(t)
	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "gone", Namespace: "default"},
	})
	if err != nil {
		t.Fatalf("a deleted application must not produce an error: %v", err)
	}
	if result.Requeue || result.RequeueAfter != 0 {
		t.Errorf("a deleted application must not be requeued: %+v", result)
	}
}

func TestReconcileHonoursDisabledExposure(t *testing.T) {
	app := sampleApp(func(a *brunerv1alpha1.BrunerApp) {
		a.Spec.LocalExposure = &brunerv1alpha1.LocalExposure{Enabled: ptr.To(false)}
	})
	reconciler, fakeClient := newReconciler(t, app)
	ctx := context.Background()

	if _, err := reconciler.Reconcile(ctx, request(app)); err != nil {
		t.Fatalf("reconciliation failed: %v", err)
	}

	stored := &brunerv1alpha1.BrunerApp{}
	key := types.NamespacedName{Name: "showroom", Namespace: "brunernetes-demo"}
	if err := fakeClient.Get(ctx, key, stored); err != nil {
		t.Fatalf("the application could not be read: %v", err)
	}
	if stored.Status.LocalEndpoint != "" {
		t.Errorf("no local endpoint must be published: got %q", stored.Status.LocalEndpoint)
	}
	if stored.Status.ClusterIPEndpoint == "" {
		t.Error("the in-cluster endpoint must still be reported")
	}
}
