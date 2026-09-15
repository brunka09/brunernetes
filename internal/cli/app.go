package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"

	brunerv1alpha1 "github.com/brunka09/brunernetes/api/v1alpha1"
)

// DecodeBrunerApp reads a BrunerApp manifest from a file, or from standard
// input when the path is "-".
func DecodeBrunerApp(path string) (*brunerv1alpha1.BrunerApp, error) {
	app := &brunerv1alpha1.BrunerApp{}
	if err := decodeManifest(path, app); err != nil {
		return nil, err
	}
	if app.Kind != "" && app.Kind != "BrunerApp" {
		return nil, fmt.Errorf(
			"manifest %q declares kind %q but a BrunerApp was expected. Point the command at an application manifest. Wrong work order for this line",
			path, app.Kind)
	}
	if app.Name == "" {
		return nil, fmt.Errorf(
			"manifest %q has no metadata.name. Every application needs a name. The work order is unsigned", path)
	}
	if app.Spec.Image == "" {
		return nil, fmt.Errorf(
			"manifest %q has no spec.image. Brunernetes deploys existing images only. The parts list is empty", path)
	}
	if app.Namespace == "" {
		app.Namespace = "default"
	}
	return app, nil
}

// DecodeKonveer reads a Konveer manifest from a file, or from standard input
// when the path is "-".
func DecodeKonveer(path string) (*brunerv1alpha1.Konveer, error) {
	konveer := &brunerv1alpha1.Konveer{}
	if err := decodeManifest(path, konveer); err != nil {
		return nil, err
	}
	if konveer.Kind != "" && konveer.Kind != "Konveer" {
		return nil, fmt.Errorf(
			"manifest %q declares kind %q but a Konveer was expected. Point the command at a delivery manifest. Wrong work order for this line",
			path, konveer.Kind)
	}
	if konveer.Name == "" {
		return nil, fmt.Errorf("manifest %q has no metadata.name. The delivery order is unsigned", path)
	}
	if konveer.Spec.AppRef == "" {
		return nil, fmt.Errorf("manifest %q has no spec.appRef. A delivery needs a target application. The delivery order has no destination", path)
	}
	if konveer.Spec.Image == "" {
		return nil, fmt.Errorf("manifest %q has no spec.image. Brunernetes delivers existing images only. The parts list is empty", path)
	}
	if konveer.Namespace == "" {
		konveer.Namespace = "default"
	}
	return konveer, nil
}

// decodeManifest reads a single YAML or JSON document into an object.
func decodeManifest(path string, into runtime.Object) error {
	var content []byte
	var err error
	if path == "-" {
		content, err = io.ReadAll(os.Stdin)
	} else {
		content, err = os.ReadFile(path)
	}
	if err != nil {
		return fmt.Errorf("manifest %q could not be read: %w. The work order never arrived at the gate", path, err)
	}
	if len(content) == 0 {
		return fmt.Errorf("manifest %q is empty. The work order is blank", path)
	}
	if err := yaml.Unmarshal(content, into); err != nil {
		return fmt.Errorf("manifest %q could not be parsed: %w. Check the YAML syntax. The work order is unreadable", path, err)
	}
	return nil
}

// DeployApp applies a BrunerApp and optionally waits until it is available.
func DeployApp(ctx context.Context, kubeClient client.Client, printer *Printer, app *brunerv1alpha1.BrunerApp, wait time.Duration) error {
	existing := &brunerv1alpha1.BrunerApp{}
	err := kubeClient.Get(ctx, types.NamespacedName{Name: app.Name, Namespace: app.Namespace}, existing)
	switch {
	case apierrors.IsNotFound(err):
		if err := kubeClient.Create(ctx, app); err != nil {
			return fmt.Errorf(
				"BrunerApp %q could not be created: %w. Check the CRDs and your permissions. The work order was rejected at the gate",
				app.Name, err)
		}
		printer.OK("BrunerApp %q created in namespace %q with image %q. Work order accepted at the gate.", app.Name, app.Namespace, app.Spec.Image)
	case err != nil:
		return fmt.Errorf("BrunerApp %q could not be read: %w. The product register is unreachable", app.Name, err)
	default:
		existing.Spec = app.Spec
		if len(app.Labels) > 0 {
			existing.Labels = mergeMaps(existing.Labels, app.Labels)
		}
		if len(app.Annotations) > 0 {
			existing.Annotations = mergeMaps(existing.Annotations, app.Annotations)
		}
		if err := kubeClient.Update(ctx, existing); err != nil {
			return fmt.Errorf(
				"BrunerApp %q could not be updated: %w. Check your permissions. The change order was rejected",
				app.Name, err)
		}
		printer.OK("BrunerApp %q updated in namespace %q with image %q. Change order accepted.", app.Name, app.Namespace, app.Spec.Image)
	}

	if wait <= 0 {
		printer.Info("Follow the rollout with: brunerctl app inspect %s", app.Name)
		return nil
	}
	return WaitForApp(ctx, kubeClient, printer, app.Name, app.Namespace, wait)
}

// WaitForApp polls the application until it becomes available, a blocking fault
// is reported, or the timeout expires.
func WaitForApp(ctx context.Context, kubeClient client.Client, printer *Printer, name, namespace string, timeout time.Duration) error {
	printer.Info("Waiting up to %s for the application to become available. Assembly is in progress.", timeout)

	deadline := time.Now().Add(timeout)
	lastMessage := ""
	for time.Now().Before(deadline) {
		app := &brunerv1alpha1.BrunerApp{}
		if err := kubeClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, app); err != nil {
			return fmt.Errorf("BrunerApp %q could not be read while waiting: %w", name, err)
		}

		available := meta.FindStatusCondition(app.Status.Conditions, brunerv1alpha1.ConditionAvailable)
		degraded := meta.FindStatusCondition(app.Status.Conditions, brunerv1alpha1.ConditionDegraded)

		if degraded != nil && degraded.Status == metav1.ConditionTrue && degraded.Message != lastMessage {
			lastMessage = degraded.Message
			printer.Fail("%s", degraded.Message)
		} else if available != nil && available.Message != lastMessage {
			lastMessage = available.Message
			printer.Info("%s", available.Message)
		}

		if available != nil && available.Status == metav1.ConditionTrue {
			printer.OK("Application %q is available at %s.", name, firstNonEmpty(app.Status.LocalEndpoint, app.Status.ClusterIPEndpoint, "the cluster endpoint"))
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
	return fmt.Errorf(
		"application %q did not become available within %s. Inspect it with: brunerctl app inspect %s. The unit is still on the line at the end of the shift",
		name, timeout, name)
}

// ListApps prints every application in the cluster or in one namespace.
func ListApps(ctx context.Context, kubeClient client.Client, printer *Printer, namespace string) error {
	printer.Section("Applications")
	return listApps(ctx, kubeClient, printer, namespace)
}

// listApps renders the application table without a section header.
func listApps(ctx context.Context, kubeClient client.Client, printer *Printer, namespace string) error {
	list := &brunerv1alpha1.BrunerAppList{}
	options := []client.ListOption{}
	if namespace != "" {
		options = append(options, client.InNamespace(namespace))
	}
	if err := kubeClient.List(ctx, list, options...); err != nil {
		printer.Fail("BrunerApp resources could not be read: %v. The Brunernetes CRDs may not be installed. The product register is missing.", err)
		return err
	}
	if len(list.Items) == 0 {
		printer.Info("No application is deployed. Deploy one with: brunerctl app deploy -f app.yaml. The line is empty.")
		return nil
	}

	rows := make([][]string, 0, len(list.Items))
	for i := range list.Items {
		app := &list.Items[i]
		available := "Unknown"
		if condition := meta.FindStatusCondition(app.Status.Conditions, brunerv1alpha1.ConditionAvailable); condition != nil {
			available = string(condition.Status)
		}
		rows = append(rows, []string{
			app.Namespace,
			app.Name,
			truncate(app.Spec.Image, 44),
			string(app.Status.Stage),
			fmt.Sprintf("%d/%d", app.Status.ReadyReplicas, app.Status.Replicas),
			available,
			firstNonEmpty(app.Status.LocalEndpoint, "-"),
		})
	}
	printer.Table([]string{"NAMESPACE", "NAME", "IMAGE", "STAGE", "READY", "AVAILABLE", "LOCAL ENDPOINT"}, rows)
	return nil
}

// InspectApp prints everything known about one application, including the
// objects the controller owns and the pod level faults behind a stalled
// rollout.
func InspectApp(ctx context.Context, kubeClient client.Client, printer *Printer, name, namespace string) error {
	app := &brunerv1alpha1.BrunerApp{}
	if err := kubeClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, app); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf(
				"BrunerApp %q was not found in namespace %q. List the applications with: brunerctl app list. No such work order in the register",
				name, namespace)
		}
		return fmt.Errorf("BrunerApp %q could not be read: %w", name, err)
	}

	printer.Section("Application " + app.Namespace + "/" + app.Name)
	mode := brunerv1alpha1.MergeModes(app.Status.EffectiveMode, app.Spec.Mode)
	printer.KeyValues([][2]string{
		{"Image", app.Spec.Image},
		{"Applied image", firstNonEmpty(app.Status.AppliedImage, "not applied yet")},
		{"Stage", firstNonEmpty(string(app.Status.Stage), "not reported")},
		{"Replicas ready", fmt.Sprintf("%d/%d", app.Status.ReadyReplicas, app.Status.Replicas)},
		{"Profile", string(mode.Profile)},
		{"Quality gate", string(mode.QualityGate)},
		{"Shift", string(mode.Shift)},
		{"Local endpoint", firstNonEmpty(app.Status.LocalEndpoint, "not published")},
		{"Cluster endpoint", firstNonEmpty(app.Status.ClusterIPEndpoint, "not published")},
		{"Deployment", firstNonEmpty(app.Status.DeploymentName, "not created yet")},
		{"Service", firstNonEmpty(app.Status.ServiceName, "not created yet")},
	})

	printer.Section("Conditions")
	printConditions(printer, app.Status.Conditions)

	printer.Section("Pods")
	pods := &corev1.PodList{}
	if err := kubeClient.List(ctx, pods,
		client.InNamespace(app.Namespace),
		client.MatchingLabels(map[string]string{"app.kubernetes.io/instance": app.Name}),
	); err != nil {
		printer.Warn("Pods could not be listed: %v.", err)
	} else if len(pods.Items) == 0 {
		printer.Info("No pod exists yet. The controller may not have created the Deployment. Nothing is on the line.")
	} else {
		rows := make([][]string, 0, len(pods.Items))
		for i := range pods.Items {
			pod := &pods.Items[i]
			ready := 0
			restarts := int32(0)
			detail := ""
			for _, status := range pod.Status.ContainerStatuses {
				if status.Ready {
					ready++
				}
				restarts += status.RestartCount
				if status.State.Waiting != nil {
					detail = status.State.Waiting.Reason
				}
			}
			rows = append(rows, []string{
				pod.Name,
				string(pod.Status.Phase),
				fmt.Sprintf("%d/%d", ready, len(pod.Spec.Containers)),
				fmt.Sprintf("%d", restarts),
				firstNonEmpty(detail, "-"),
				formatAge(pod.CreationTimestamp.Time),
			})
		}
		printer.Table([]string{"POD", "PHASE", "READY", "RESTARTS", "WAITING", "AGE"}, rows)
	}

	printer.Section("Owned objects")
	deployment := &appsv1.Deployment{}
	if err := kubeClient.Get(ctx, types.NamespacedName{Name: app.Name, Namespace: app.Namespace}, deployment); err == nil {
		ready, desired := DeploymentReady(deployment)
		printer.OK("Deployment %q reports %d of %d replicas ready.", deployment.Name, ready, desired)
	} else {
		printer.Warn("Deployment %q could not be read: %v.", app.Name, err)
	}
	service := &corev1.Service{}
	if err := kubeClient.Get(ctx, types.NamespacedName{Name: app.Name, Namespace: app.Namespace}, service); err == nil {
		printer.OK("Service %q of type %s has cluster IP %s.", service.Name, service.Spec.Type, service.Spec.ClusterIP)
	} else {
		printer.Warn("Service %q could not be read: %v.", app.Name, err)
	}
	return nil
}

// mergeMaps copies the entries of an overlay over a base map.
func mergeMaps(base, overlay map[string]string) map[string]string {
	if base == nil {
		base = map[string]string{}
	}
	for key, value := range overlay {
		base[key] = value
	}
	return base
}
