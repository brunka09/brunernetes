package factory

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	brunerv1alpha1 "github.com/brunka09/brunernetes/api/v1alpha1"
)

// Factory defaults. A single-node host is small, so the defaults are modest and
// are only applied where the specification leaves a value unset.
const (
	// DefaultPort is the container port used when the specification omits one.
	DefaultPort int32 = 8080
	// DefaultHealthPath is the probe path used when the specification omits one.
	DefaultHealthPath = "/healthz"
	// DefaultHostIP is the loopback address applications are published on.
	DefaultHostIP = "127.0.0.1"
	// DefaultHostPort is the loopback port applications are published on.
	DefaultHostPort int32 = 30080
	// ServicePort is the port exposed by the ClusterIP Service.
	ServicePort int32 = 80
	// PortName is the name of the HTTP port on the pod and the Service.
	PortName = "http"
	// DefaultRevisionHistoryLimit bounds stored ReplicaSets on a small host.
	DefaultRevisionHistoryLimit int32 = 3
	// DefaultProgressDeadlineSeconds bounds a stuck rollout.
	DefaultProgressDeadlineSeconds int32 = 300
)

// DefaultResources returns the resource requests and limits applied to a
// container that does not declare its own.
func DefaultResources() corev1.ResourceRequirements {
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("50m"),
			corev1.ResourceMemory: resource.MustParse("64Mi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("500m"),
			corev1.ResourceMemory: resource.MustParse("256Mi"),
		},
	}
}

// ResolvedPort returns the container port of the application.
func ResolvedPort(app *brunerv1alpha1.BrunerApp) int32 {
	if app.Spec.Port > 0 {
		return app.Spec.Port
	}
	return DefaultPort
}

// ResolvedHealthPath returns the probe path of the application.
func ResolvedHealthPath(app *brunerv1alpha1.BrunerApp) string {
	if app.Spec.HealthPath != "" {
		return app.Spec.HealthPath
	}
	return DefaultHealthPath
}

// ResolvedReplicas returns the desired replica count of the application.
func ResolvedReplicas(app *brunerv1alpha1.BrunerApp) int32 {
	if app.Spec.Replicas != nil {
		return *app.Spec.Replicas
	}
	return 1
}

// ResolvedExposure returns the effective loopback exposure configuration. A
// non-loopback host IP is never honoured: the factory falls back to 127.0.0.1
// because Brunernetes only ever publishes an application locally.
func ResolvedExposure(app *brunerv1alpha1.BrunerApp) brunerv1alpha1.LocalExposure {
	exposure := brunerv1alpha1.LocalExposure{
		HostIP:   DefaultHostIP,
		HostPort: DefaultHostPort,
	}
	enabled := true
	exposure.Enabled = &enabled

	if app.Spec.LocalExposure == nil {
		return exposure
	}
	if app.Spec.LocalExposure.Enabled != nil {
		value := *app.Spec.LocalExposure.Enabled
		exposure.Enabled = &value
	}
	if brunerv1alpha1.IsLoopbackHostIP(app.Spec.LocalExposure.HostIP) {
		exposure.HostIP = app.Spec.LocalExposure.HostIP
	}
	if app.Spec.LocalExposure.HostPort > 0 {
		exposure.HostPort = app.Spec.LocalExposure.HostPort
	}
	return exposure
}

// ExposureEnabled reports whether loopback publishing is requested.
func ExposureEnabled(app *brunerv1alpha1.BrunerApp) bool {
	exposure := ResolvedExposure(app)
	return exposure.Enabled != nil && *exposure.Enabled
}

// LocalEndpoint renders the loopback endpoint of the application, or an empty
// string when local exposure is disabled.
func LocalEndpoint(app *brunerv1alpha1.BrunerApp) string {
	if !ExposureEnabled(app) {
		return ""
	}
	exposure := ResolvedExposure(app)
	return fmt.Sprintf("http://%s:%d", exposure.HostIP, exposure.HostPort)
}

// ClusterIPEndpoint renders the in-cluster endpoint of the application.
func ClusterIPEndpoint(app *brunerv1alpha1.BrunerApp) string {
	return fmt.Sprintf("http://%s.%s.svc.cluster.local:%d", ServiceName(app), app.Namespace, ServicePort)
}

// MergeResources fills only the resource values the specification leaves unset.
// A value provided by the user is never replaced.
func MergeResources(spec *corev1.ResourceRequirements) corev1.ResourceRequirements {
	defaults := DefaultResources()
	merged := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{},
		Limits:   corev1.ResourceList{},
	}
	if spec != nil {
		for name, quantity := range spec.Requests {
			merged.Requests[name] = quantity
		}
		for name, quantity := range spec.Limits {
			merged.Limits[name] = quantity
		}
	}
	for name, quantity := range defaults.Requests {
		if _, ok := merged.Requests[name]; !ok {
			merged.Requests[name] = quantity
		}
	}
	for name, quantity := range defaults.Limits {
		if _, ok := merged.Limits[name]; !ok {
			merged.Limits[name] = quantity
		}
	}
	return merged
}

// DefaultReadinessProbe returns the readiness probe applied when the
// specification does not provide one.
func DefaultReadinessProbe(port int32, path string) *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: path,
				Port: intstr.FromInt32(port),
			},
		},
		InitialDelaySeconds: 2,
		PeriodSeconds:       5,
		TimeoutSeconds:      2,
		SuccessThreshold:    1,
		FailureThreshold:    3,
	}
}

// DefaultLivenessProbe returns the liveness probe applied when the
// specification does not provide one. It is deliberately more forgiving than
// the readiness probe so a slow container is not restarted needlessly.
func DefaultLivenessProbe(port int32, path string) *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: path,
				Port: intstr.FromInt32(port),
			},
		},
		InitialDelaySeconds: 10,
		PeriodSeconds:       20,
		TimeoutSeconds:      3,
		SuccessThreshold:    1,
		FailureThreshold:    5,
	}
}

// RolloutStrategy returns the rollout strategy of the factory. "steady" is
// surge free: on a single small node it is better to replace a replica than to
// ask for capacity the host may not have.
func RolloutStrategy() appsv1.DeploymentStrategy {
	maxSurge := intstr.FromInt32(0)
	maxUnavailable := intstr.FromInt32(1)
	return appsv1.DeploymentStrategy{
		Type: appsv1.RollingUpdateDeploymentStrategyType,
		RollingUpdate: &appsv1.RollingUpdateDeployment{
			MaxSurge:       &maxSurge,
			MaxUnavailable: &maxUnavailable,
		},
	}
}

// MinReadySeconds returns how long a new replica must stay ready before it
// counts as available. The showroom profile waits a little longer so a
// demonstration does not flip between states.
func MinReadySeconds(mode brunerv1alpha1.BrunerMode) int32 {
	if mode.Profile == brunerv1alpha1.ProfileShowroom {
		return 5
	}
	return 0
}

// ContainerSecurityContext returns a conservative security context that works
// with arbitrary images. It does not force a non-root user, because the image
// is supplied by the user and may legitimately define its own.
func ContainerSecurityContext() *corev1.SecurityContext {
	allowPrivilegeEscalation := false
	privileged := false
	return &corev1.SecurityContext{
		AllowPrivilegeEscalation: &allowPrivilegeEscalation,
		Privileged:               &privileged,
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{"ALL"},
		},
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
	}
}

// BuildDeployment renders the Deployment owned by an application. The function
// is deterministic: the same inputs always produce the same object, which is
// what makes the reconciliation idempotent.
func BuildDeployment(app *brunerv1alpha1.BrunerApp, mode brunerv1alpha1.BrunerMode) *appsv1.Deployment {
	port := ResolvedPort(app)
	healthPath := ResolvedHealthPath(app)
	replicas := ResolvedReplicas(app)
	labels := ObjectLabels(app, mode)
	selector := SelectorLabels(app)
	resources := MergeResources(app.Spec.Resources)

	readiness := app.Spec.ReadinessProbe.DeepCopy()
	if readiness == nil {
		readiness = DefaultReadinessProbe(port, healthPath)
	}
	liveness := app.Spec.LivenessProbe.DeepCopy()
	if liveness == nil {
		liveness = DefaultLivenessProbe(port, healthPath)
	}

	containerPort := corev1.ContainerPort{
		Name:          PortName,
		ContainerPort: port,
		Protocol:      corev1.ProtocolTCP,
	}
	if ExposureEnabled(app) {
		exposure := ResolvedExposure(app)
		containerPort.HostIP = exposure.HostIP
		containerPort.HostPort = exposure.HostPort
	}

	podAnnotations := map[string]string{
		AnnotationAppliedImage: app.Spec.Image,
		AnnotationQualityGate:  string(mode.QualityGate),
	}
	if endpoint := LocalEndpoint(app); endpoint != "" {
		podAnnotations[AnnotationLocalEndpoint] = endpoint
	}

	revisionHistoryLimit := DefaultRevisionHistoryLimit
	progressDeadline := DefaultProgressDeadlineSeconds
	replicaCount := replicas

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      DeploymentName(app),
			Namespace: app.Namespace,
			Labels:    labels,
			Annotations: map[string]string{
				AnnotationAppliedImage: app.Spec.Image,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas:                &replicaCount,
			RevisionHistoryLimit:    &revisionHistoryLimit,
			ProgressDeadlineSeconds: &progressDeadline,
			MinReadySeconds:         MinReadySeconds(mode),
			Selector:                &metav1.LabelSelector{MatchLabels: selector},
			Strategy:                RolloutStrategy(),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      labels,
					Annotations: podAnnotations,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: app.Spec.ServiceAccountName,
					ImagePullSecrets:   app.Spec.ImagePullSecrets,
					Containers: []corev1.Container{
						{
							Name:            "application",
							Image:           app.Spec.Image,
							ImagePullPolicy: app.Spec.ImagePullPolicy,
							Ports:           []corev1.ContainerPort{containerPort},
							Env:             app.Spec.Env,
							Resources:       resources,
							ReadinessProbe:  readiness,
							LivenessProbe:   liveness,
							SecurityContext: ContainerSecurityContext(),
						},
					},
				},
			},
		},
	}
}

// BuildService renders the ClusterIP Service owned by an application. The
// factory never creates a NodePort or LoadBalancer Service: local exposure is
// handled by a loopback host port on the pod.
func BuildService(app *brunerv1alpha1.BrunerApp, mode brunerv1alpha1.BrunerMode) *corev1.Service {
	labels := ObjectLabels(app, mode)
	annotations := map[string]string{}
	if endpoint := LocalEndpoint(app); endpoint != "" {
		annotations[AnnotationLocalEndpoint] = endpoint
	}

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        ServiceName(app),
			Namespace:   app.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: SelectorLabels(app),
			Ports: []corev1.ServicePort{
				{
					Name:       PortName,
					Port:       ServicePort,
					TargetPort: intstr.FromString(PortName),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}
}
