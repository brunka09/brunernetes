package factory

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	brunerv1alpha1 "github.com/brunka09/brunernetes/api/v1alpha1"
)

// testApp returns a minimal application used by the build tests.
func testApp(mutators ...func(*brunerv1alpha1.BrunerApp)) *brunerv1alpha1.BrunerApp {
	app := &brunerv1alpha1.BrunerApp{
		ObjectMeta: metav1.ObjectMeta{Name: "showroom", Namespace: "brunernetes-demo"},
		Spec:       brunerv1alpha1.BrunerAppSpec{Image: "ghcr.io/brunka09/showroom:v0.1.0"},
	}
	for _, mutate := range mutators {
		mutate(app)
	}
	return app
}

func TestResolvedDefaults(t *testing.T) {
	app := testApp()
	if got := ResolvedPort(app); got != DefaultPort {
		t.Errorf("port: got %d, want %d", got, DefaultPort)
	}
	if got := ResolvedHealthPath(app); got != DefaultHealthPath {
		t.Errorf("health path: got %q, want %q", got, DefaultHealthPath)
	}
	if got := ResolvedReplicas(app); got != 1 {
		t.Errorf("replicas: got %d, want 1", got)
	}

	custom := testApp(func(a *brunerv1alpha1.BrunerApp) {
		a.Spec.Port = 9000
		a.Spec.HealthPath = "/live"
		a.Spec.Replicas = ptr.To(int32(3))
	})
	if got := ResolvedPort(custom); got != 9000 {
		t.Errorf("explicit port must win: got %d", got)
	}
	if got := ResolvedHealthPath(custom); got != "/live" {
		t.Errorf("explicit health path must win: got %q", got)
	}
	if got := ResolvedReplicas(custom); got != 3 {
		t.Errorf("explicit replica count must win: got %d", got)
	}
}

func TestResolvedExposureRejectsNonLoopbackAddress(t *testing.T) {
	app := testApp(func(a *brunerv1alpha1.BrunerApp) {
		a.Spec.LocalExposure = &brunerv1alpha1.LocalExposure{
			Enabled:  ptr.To(true),
			HostIP:   "0.0.0.0",
			HostPort: 31000,
		}
	})
	exposure := ResolvedExposure(app)
	if exposure.HostIP != DefaultHostIP {
		t.Errorf("a non-loopback host IP must fall back to %s: got %s", DefaultHostIP, exposure.HostIP)
	}
	if exposure.HostPort != 31000 {
		t.Errorf("an explicit host port must be honoured: got %d", exposure.HostPort)
	}

	disabled := testApp(func(a *brunerv1alpha1.BrunerApp) {
		a.Spec.LocalExposure = &brunerv1alpha1.LocalExposure{Enabled: ptr.To(false)}
	})
	if ExposureEnabled(disabled) {
		t.Error("exposure must be reported as disabled")
	}
	if got := LocalEndpoint(disabled); got != "" {
		t.Errorf("a disabled exposure must not publish an endpoint: got %q", got)
	}
	if got := LocalEndpoint(testApp()); got != "http://127.0.0.1:30080" {
		t.Errorf("default endpoint: got %q", got)
	}
	if got := ClusterIPEndpoint(testApp()); got != "http://showroom.brunernetes-demo.svc.cluster.local:80" {
		t.Errorf("cluster endpoint: got %q", got)
	}
}

func TestBuildDeploymentIsDeterministic(t *testing.T) {
	app := testApp()
	mode := brunerv1alpha1.DefaultMode()

	first := BuildDeployment(app, mode)
	second := BuildDeployment(app, mode)
	if !reflect.DeepEqual(first, second) {
		t.Fatal("BuildDeployment must be deterministic: two calls produced different objects")
	}

	if first.Name != "showroom" || first.Namespace != "brunernetes-demo" {
		t.Errorf("unexpected object meta: %s/%s", first.Namespace, first.Name)
	}
	if !reflect.DeepEqual(first.Spec.Selector.MatchLabels, SelectorLabels(app)) {
		t.Error("the Deployment selector must use the immutable selector labels")
	}
	if got := *first.Spec.Replicas; got != 1 {
		t.Errorf("replicas: got %d, want 1", got)
	}

	container := first.Spec.Template.Spec.Containers[0]
	if container.Image != app.Spec.Image {
		t.Errorf("image: got %q, want %q", container.Image, app.Spec.Image)
	}
	if container.ReadinessProbe == nil || container.LivenessProbe == nil {
		t.Fatal("default probes must be applied when the specification omits them")
	}
	if got := container.ReadinessProbe.HTTPGet.Path; got != DefaultHealthPath {
		t.Errorf("readiness path: got %q, want %q", got, DefaultHealthPath)
	}
	if len(container.Ports) != 1 {
		t.Fatalf("exactly one container port is expected, got %d", len(container.Ports))
	}
	port := container.Ports[0]
	if port.ContainerPort != DefaultPort || port.Name != PortName {
		t.Errorf("container port: got %d/%q", port.ContainerPort, port.Name)
	}
	if port.HostIP != DefaultHostIP || port.HostPort != DefaultHostPort {
		t.Errorf("loopback publishing: got %s:%d", port.HostIP, port.HostPort)
	}
	if container.SecurityContext == nil || *container.SecurityContext.AllowPrivilegeEscalation {
		t.Error("privilege escalation must be disabled on the container")
	}
}

func TestBuildDeploymentRolloutIsSurgeFree(t *testing.T) {
	deployment := BuildDeployment(testApp(), brunerv1alpha1.DefaultMode())
	strategy := deployment.Spec.Strategy
	if strategy.Type != "RollingUpdate" || strategy.RollingUpdate == nil {
		t.Fatalf("the steady rollout must be a RollingUpdate: got %q", strategy.Type)
	}
	if got := strategy.RollingUpdate.MaxSurge.IntValue(); got != 0 {
		t.Errorf("a single small node must not surge: maxSurge is %d", got)
	}
	if got := strategy.RollingUpdate.MaxUnavailable.IntValue(); got != 1 {
		t.Errorf("maxUnavailable: got %d, want 1", got)
	}
}

func TestBuildDeploymentDisablesHostPortWhenExposureIsOff(t *testing.T) {
	app := testApp(func(a *brunerv1alpha1.BrunerApp) {
		a.Spec.LocalExposure = &brunerv1alpha1.LocalExposure{Enabled: ptr.To(false)}
	})
	container := BuildDeployment(app, brunerv1alpha1.DefaultMode()).Spec.Template.Spec.Containers[0]
	if container.Ports[0].HostPort != 0 {
		t.Errorf("no host port must be requested when exposure is disabled: got %d", container.Ports[0].HostPort)
	}
}

func TestBuildServiceIsAlwaysClusterIP(t *testing.T) {
	app := testApp(func(a *brunerv1alpha1.BrunerApp) {
		a.Spec.LocalExposure = &brunerv1alpha1.LocalExposure{Enabled: ptr.To(true), HostPort: 31111}
	})
	service := BuildService(app, brunerv1alpha1.DefaultMode())
	if service.Spec.Type != corev1.ServiceTypeClusterIP {
		t.Errorf("the factory must never create a %s Service", service.Spec.Type)
	}
	if !reflect.DeepEqual(service.Spec.Selector, SelectorLabels(app)) {
		t.Error("the Service selector must use the immutable selector labels")
	}
	if len(service.Spec.Ports) != 1 || service.Spec.Ports[0].Port != ServicePort {
		t.Errorf("the Service must expose port %d: got %+v", ServicePort, service.Spec.Ports)
	}
	if service.Spec.Ports[0].TargetPort.String() != PortName {
		t.Errorf("the Service must target the named container port: got %v", service.Spec.Ports[0].TargetPort)
	}
	if !reflect.DeepEqual(service, BuildService(app, brunerv1alpha1.DefaultMode())) {
		t.Error("BuildService must be deterministic")
	}
}

func TestMergeResourcesKeepsUserValues(t *testing.T) {
	if got := MergeResources(nil); !reflect.DeepEqual(got, DefaultResources()) {
		t.Errorf("an unset specification must produce the defaults: got %+v", got)
	}

	userCPU := resource.MustParse("500m")
	merged := MergeResources(&corev1.ResourceRequirements{
		Requests: corev1.ResourceList{corev1.ResourceCPU: userCPU},
	})
	if got := merged.Requests[corev1.ResourceCPU]; got.Cmp(userCPU) != 0 {
		t.Errorf("a user supplied CPU request must never be replaced: got %s", got.String())
	}
	if _, ok := merged.Requests[corev1.ResourceMemory]; !ok {
		t.Error("an unset memory request must be filled from the defaults")
	}
	if _, ok := merged.Limits[corev1.ResourceCPU]; !ok {
		t.Error("an unset CPU limit must be filled from the defaults")
	}
}

func TestSelectorLabelsAreStable(t *testing.T) {
	app := testApp()
	want := map[string]string{
		LabelName:     "showroom",
		LabelInstance: "showroom",
		LabelPartOf:   PartOfName,
	}
	if got := SelectorLabels(app); !reflect.DeepEqual(got, want) {
		t.Errorf("selector labels changed: got %v, want %v", got, want)
	}

	labels := ObjectLabels(app, brunerv1alpha1.DefaultMode())
	for key, value := range SelectorLabels(app) {
		if labels[key] != value {
			t.Errorf("object labels must contain the selector label %q", key)
		}
	}
	if labels[LabelProfile] != string(brunerv1alpha1.ProfileGarage) {
		t.Errorf("the effective profile must be recorded on owned objects: got %q", labels[LabelProfile])
	}
}

func TestNamesFollowTheApplication(t *testing.T) {
	app := testApp()
	if got := DeploymentName(app); got != "showroom" {
		t.Errorf("deployment name: got %q", got)
	}
	if got := ServiceName(app); got != "showroom" {
		t.Errorf("service name: got %q", got)
	}
}

func TestMinReadySecondsFavoursTheShowroom(t *testing.T) {
	garage := MinReadySeconds(brunerv1alpha1.BrunerMode{Profile: brunerv1alpha1.ProfileGarage})
	showroom := MinReadySeconds(brunerv1alpha1.BrunerMode{Profile: brunerv1alpha1.ProfileShowroom})
	if showroom <= garage {
		t.Errorf("the showroom profile must settle longer than the garage profile: %d vs %d", showroom, garage)
	}
}
