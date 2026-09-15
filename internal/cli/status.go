package cli

import (
	"context"
	"fmt"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	brunerv1alpha1 "github.com/brunka09/brunernetes/api/v1alpha1"
	"github.com/brunka09/brunernetes/internal/factory"
)

// Status prints a short overview of the whole factory: the node, the control
// plane, the policy and the application.
func Status(ctx context.Context, kubeClient client.Client, printer *Printer) error {
	printer.Section("Factory status")

	node, err := SingleNode(ctx, kubeClient)
	if err != nil {
		printer.Fail("%v", err)
		return err
	}

	nodeState := "NotReady"
	if NodeReady(node) {
		nodeState = "Ready"
	}
	printer.KeyValues([][2]string{
		{"Node", node.Name},
		{"Node state", nodeState},
		{"Kubernetes", node.Status.NodeInfo.KubeletVersion},
		{"Container runtime", node.Status.NodeInfo.ContainerRuntimeVersion},
		{"Operating system", node.Status.NodeInfo.OSImage},
		{"CPU allocatable", node.Status.Allocatable.Cpu().String()},
		{"Memory allocatable", node.Status.Allocatable.Memory().String()},
		{"Schedulable", boolLabel(!node.Spec.Unschedulable && !hasControlPlaneTaint(node))},
	})

	if !NodeReady(node) {
		printer.Fail("Node is NotReady. Check kubelet and containerd. Factory floor unavailable.")
	} else if hasControlPlaneTaint(node) {
		printer.Warn("Node still carries the control-plane taint, so ordinary workloads stay Pending. Remove it with: kubectl taint nodes --all node-role.kubernetes.io/control-plane-. The floor is fenced off.")
	} else {
		printer.OK("Node is Ready and accepts workloads. The factory floor is open.")
	}

	printer.Section("Factory policy")
	policy, policyName := readPolicy(ctx, kubeClient)
	printer.KeyValues([][2]string{
		{"AvtoVAZ resource", policyName},
		{"Latest tag policy", string(policy.LatestTagPolicy)},
		{"Readiness probe required", boolLabel(policy.RequireReadinessProbe != nil && *policy.RequireReadinessProbe)},
		{"Resource requests required", boolLabel(policy.RequireResourceRequests != nil && *policy.RequireResourceRequests)},
		{"Maximum applications", fmt.Sprintf("%d", policy.MaxApplications)},
	})

	printer.Section("Applications")
	return listApps(ctx, kubeClient, printer, "")
}

// FactoryStatus prints the Togliatti resources and the underlying environment.
func FactoryStatus(ctx context.Context, kubeClient client.Client, printer *Printer) error {
	printer.Section("Togliatti factory environment")

	list := &brunerv1alpha1.TogliattiList{}
	if err := kubeClient.List(ctx, list); err != nil {
		printer.Fail("Togliatti resources could not be read: %v. The Brunernetes CRDs may not be installed. The factory register is missing.", err)
		return err
	}
	if len(list.Items) == 0 {
		printer.Warn("No Togliatti resource exists. Apply config/samples/bruner.dev_v1alpha1_togliatti.yaml to register this factory. The plant is not on the map.")
	}
	for i := range list.Items {
		item := &list.Items[i]
		printer.KeyValues([][2]string{
			{"Name", item.Name},
			{"Node", firstNonEmpty(item.Status.NodeName, item.Spec.NodeName, "not observed")},
			{"Expected Kubernetes", firstNonEmpty(item.Spec.ExpectedKubernetesVersion, "unset")},
			{"Observed Kubernetes", firstNonEmpty(item.Status.KubernetesVersion, "not observed")},
			{"Capacity profile", firstNonEmpty(string(item.Status.ActiveCapacityProfile), string(item.Spec.CapacityProfile), "not observed")},
		})
		printConditions(printer, item.Status.Conditions)
	}

	// The Togliatti controller lands in v0.2, so the CLI reports the live
	// environment directly and says so.
	printer.Section("Live environment")
	node, err := SingleNode(ctx, kubeClient)
	if err != nil {
		printer.Fail("%v", err)
		return err
	}
	printer.Info("Togliatti status is reported by brunerctl in this release; the in-cluster Togliatti controller is scheduled for v0.2.")

	checks := []struct {
		name string
		ok   bool
		bad  string
		good string
	}{
		{"Kubernetes API", true, "", "API server answered a node list request. The factory office is reachable."},
		{"Node readiness", NodeReady(node), "Node is NotReady. Check kubelet and containerd. Factory floor unavailable.", "Node is Ready. The floor is staffed."},
		{"Container runtime", node.Status.NodeInfo.ContainerRuntimeVersion != "", "Container runtime version is not reported by the node. Check containerd. The machine shop is silent.", "Container runtime is reported by the node: " + node.Status.NodeInfo.ContainerRuntimeVersion},
		{"Workload scheduling", !hasControlPlaneTaint(node) && !node.Spec.Unschedulable, "Node does not accept ordinary workloads because of a taint or a cordon. The floor is fenced off.", "Node accepts ordinary workloads."},
		{"Pod network", podNetworkReady(node), "Node reports a network problem in its conditions. Check the CNI configuration in /etc/cni/net.d. The conveyor rails are not connected.", "Node reports no network problem. The conveyor rails are connected."},
	}
	for _, check := range checks {
		if check.ok {
			printer.OK("%-20s %s", check.name, check.good)
		} else {
			printer.Fail("%-20s %s", check.name, check.bad)
		}
	}
	return nil
}

// Events prints the recent events of the Brunernetes resources.
func Events(ctx context.Context, kubeClient client.Client, printer *Printer, namespace string, limit int) error {
	printer.Section("Factory events")

	events := &corev1.EventList{}
	options := []client.ListOption{}
	if namespace != "" {
		options = append(options, client.InNamespace(namespace))
	}
	if err := kubeClient.List(ctx, events, options...); err != nil {
		printer.Fail("Events could not be read: %v. Check the kubeconfig and the API server. The shift log is unreadable.", err)
		return err
	}

	filtered := make([]corev1.Event, 0, len(events.Items))
	for _, event := range events.Items {
		switch event.InvolvedObject.Kind {
		case "BrunerApp", "Konveer", "Togliatti", "AvtoVAZ":
			filtered = append(filtered, event)
		}
	}
	if len(filtered) == 0 {
		printer.Info("No Brunernetes events were recorded yet. The shift log is empty.")
		return nil
	}

	sort.Slice(filtered, func(i, j int) bool {
		return eventTime(filtered[i]).After(eventTime(filtered[j]))
	})
	if limit > 0 && len(filtered) > limit {
		filtered = filtered[:limit]
	}

	rows := make([][]string, 0, len(filtered))
	for _, event := range filtered {
		rows = append(rows, []string{
			formatAge(eventTime(event)),
			event.Type,
			event.InvolvedObject.Kind + "/" + event.InvolvedObject.Name,
			event.Reason,
			truncate(event.Message, 96),
		})
	}
	printer.Table([]string{"AGE", "TYPE", "OBJECT", "REASON", "MESSAGE"}, rows)
	return nil
}

// Diagnose walks the whole installation from the node down to the application
// endpoint and reports the first thing that is broken.
func Diagnose(ctx context.Context, kubeClient client.Client, printer *Printer) error {
	printer.Section("Factory diagnosis")
	problems := 0

	// 1. API server.
	if err := APIReachable(ctx, kubeClient); err != nil {
		printer.Fail("%v", err)
		return err
	}
	printer.OK("Kubernetes API server answers requests. The factory office is reachable.")

	// 2. Node.
	node, err := SingleNode(ctx, kubeClient)
	if err != nil {
		printer.Fail("%v", err)
		return err
	}
	if !NodeReady(node) {
		problems++
		printer.Fail("Node is NotReady. Check kubelet and containerd. Factory floor unavailable.")
		printer.Info("Run: systemctl status kubelet && journalctl -u kubelet -n 100")
	} else {
		printer.OK("Node %q is Ready.", node.Name)
	}
	if hasControlPlaneTaint(node) {
		problems++
		printer.Fail("Node carries the control-plane taint, so application pods stay Pending. The floor is fenced off.")
		printer.Info("Run: kubectl taint nodes --all node-role.kubernetes.io/control-plane-")
	}

	// 3. Control-plane pods.
	pods := &corev1.PodList{}
	if err := kubeClient.List(ctx, pods, client.InNamespace("kube-system")); err != nil {
		printer.Warn("Control-plane pods could not be listed: %v.", err)
	} else {
		unhealthy := 0
		for i := range pods.Items {
			if pods.Items[i].Status.Phase != corev1.PodRunning && pods.Items[i].Status.Phase != corev1.PodSucceeded {
				unhealthy++
				printer.Fail("Control-plane pod %q is in phase %s. Inspect it with: kubectl -n kube-system describe pod %s",
					pods.Items[i].Name, pods.Items[i].Status.Phase, pods.Items[i].Name)
			}
		}
		if unhealthy == 0 {
			printer.OK("All kube-system pods are Running. Utilities are online.")
		} else {
			problems += unhealthy
		}
	}

	// 4. Controller.
	deployments := &appsv1.DeploymentList{}
	if err := kubeClient.List(ctx, deployments, client.InNamespace(HelmNamespace)); err != nil || len(deployments.Items) == 0 {
		problems++
		printer.Fail("Brunernetes controller Deployment was not found in namespace %s. Install it with: helm upgrade --install %s charts/brunernetes -n %s --create-namespace. The factory office has no supervisor.",
			HelmNamespace, HelmReleaseName, HelmNamespace)
	} else {
		for i := range deployments.Items {
			ready, desired := DeploymentReady(&deployments.Items[i])
			if ready < desired {
				problems++
				printer.Fail("Controller Deployment %q has %d of %d replicas ready. Inspect it with: kubectl -n %s logs deploy/%s. The supervisor did not report for the shift.",
					deployments.Items[i].Name, ready, desired, HelmNamespace, deployments.Items[i].Name)
			} else {
				printer.OK("Controller Deployment %q has %d of %d replicas ready.", deployments.Items[i].Name, ready, desired)
			}
		}
	}

	// 5. Applications.
	apps := &brunerv1alpha1.BrunerAppList{}
	if err := kubeClient.List(ctx, apps); err != nil {
		problems++
		printer.Fail("BrunerApp resources could not be read: %v. The CRDs may not be installed. The product register is missing.", err)
	} else if len(apps.Items) == 0 {
		printer.Info("No application is deployed yet. Deploy one with: brunerctl app deploy -f app.yaml. The line is empty.")
	} else {
		for i := range apps.Items {
			app := &apps.Items[i]
			available := meta.FindStatusCondition(app.Status.Conditions, brunerv1alpha1.ConditionAvailable)
			degraded := meta.FindStatusCondition(app.Status.Conditions, brunerv1alpha1.ConditionDegraded)
			switch {
			case degraded != nil && degraded.Status == metav1.ConditionTrue:
				problems++
				printer.Fail("Application %q is degraded: %s", app.Name, degraded.Message)
			case available != nil && available.Status == metav1.ConditionTrue:
				printer.OK("Application %q is available at %s.", app.Name, firstNonEmpty(app.Status.LocalEndpoint, app.Status.ClusterIPEndpoint, "no endpoint reported"))
			default:
				message := "Application status has not been reported yet. The controller may not be running."
				if available != nil {
					message = available.Message
				}
				printer.Warn("Application %q is not available: %s", app.Name, message)
			}
			if warning := meta.FindStatusCondition(app.Status.Conditions, brunerv1alpha1.ConditionPolicyWarning); warning != nil && warning.Status == metav1.ConditionTrue {
				printer.Warn("Application %q has a policy deviation: %s", app.Name, warning.Message)
			}
		}
	}

	printer.Section("Diagnosis summary")
	if problems == 0 {
		printer.Println("No blocking problem was found. The line is running and quality control is satisfied.")
		return nil
	}
	printer.Printf("%d blocking problem(s) were found. Resolve them in the order printed above. The line is stopped until the first item is cleared.", problems)
	return fmt.Errorf("diagnosis found %d blocking problems", problems)
}

// readPolicy returns the effective factory policy and the name of the resource
// it came from.
func readPolicy(ctx context.Context, kubeClient client.Client) (brunerv1alpha1.FactoryPolicy, string) {
	list := &brunerv1alpha1.AvtoVAZList{}
	if err := kubeClient.List(ctx, list); err != nil || len(list.Items) == 0 {
		return factory.DefaultPolicy(), "none, built-in defaults are in effect"
	}
	sort.Slice(list.Items, func(i, j int) bool { return list.Items[i].Name < list.Items[j].Name })
	selected := list.Items[0]
	for i := range list.Items {
		if list.Items[i].Name == "avtovaz" {
			selected = list.Items[i]
			break
		}
	}
	return factory.NormalizePolicy(selected.Spec.Policy), selected.Name
}

// printConditions renders a condition list.
func printConditions(printer *Printer, conditions []metav1.Condition) {
	if len(conditions) == 0 {
		printer.Info("No conditions were reported yet.")
		return
	}
	rows := make([][]string, 0, len(conditions))
	for _, condition := range conditions {
		rows = append(rows, []string{
			condition.Type,
			string(condition.Status),
			condition.Reason,
			truncate(condition.Message, 84),
		})
	}
	printer.Table([]string{"CONDITION", "STATUS", "REASON", "MESSAGE"}, rows)
}

// hasControlPlaneTaint reports whether the node refuses ordinary workloads.
func hasControlPlaneTaint(node *corev1.Node) bool {
	for _, taint := range node.Spec.Taints {
		if taint.Key == "node-role.kubernetes.io/control-plane" || taint.Key == "node-role.kubernetes.io/master" {
			if taint.Effect == corev1.TaintEffectNoSchedule || taint.Effect == corev1.TaintEffectNoExecute {
				return true
			}
		}
	}
	return false
}

// podNetworkReady reports whether the node signals a network problem.
func podNetworkReady(node *corev1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeNetworkUnavailable {
			return condition.Status != corev1.ConditionTrue
		}
	}
	return NodeReady(node)
}

// eventTime returns the most useful timestamp of an event.
func eventTime(event corev1.Event) time.Time {
	if !event.LastTimestamp.IsZero() {
		return event.LastTimestamp.Time
	}
	if event.EventTime.Time != (time.Time{}) {
		return event.EventTime.Time
	}
	return event.CreationTimestamp.Time
}

// formatAge renders a timestamp as a short age.
func formatAge(timestamp time.Time) string {
	if timestamp.IsZero() {
		return "unknown"
	}
	age := time.Since(timestamp)
	switch {
	case age < time.Minute:
		return fmt.Sprintf("%ds", int(age.Seconds()))
	case age < time.Hour:
		return fmt.Sprintf("%dm", int(age.Minutes()))
	case age < 24*time.Hour:
		return fmt.Sprintf("%dh", int(age.Hours()))
	default:
		return fmt.Sprintf("%dd", int(age.Hours()/24))
	}
}

// firstNonEmpty returns the first non-empty value.
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
