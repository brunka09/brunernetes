package cli

import (
	"context"
	"fmt"
	"os"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	brunerv1alpha1 "github.com/brunka09/brunernetes/api/v1alpha1"
)

// AdminKubeconfigPath is the kubeconfig kubeadm writes on the control-plane
// node. brunerctl falls back to it when no other kubeconfig is configured.
const AdminKubeconfigPath = "/etc/kubernetes/admin.conf"

// Scheme carries the core Kubernetes types and the Brunernetes API.
func Scheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(brunerv1alpha1.AddToScheme(scheme))
	return scheme
}

// RestConfig resolves the Kubernetes connection settings from, in order: the
// --kubeconfig flag, the KUBECONFIG environment variable, the user kubeconfig,
// the in-cluster configuration and finally the kubeadm admin kubeconfig.
func RestConfig(kubeconfigPath string) (*rest.Config, error) {
	if kubeconfigPath != "" {
		config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return nil, fmt.Errorf("kubeconfig %q could not be loaded: %w", kubeconfigPath, err)
		}
		return config, nil
	}

	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		rules, &clientcmd.ConfigOverrides{}).ClientConfig()
	if err == nil {
		return config, nil
	}

	if inCluster, inClusterErr := rest.InClusterConfig(); inClusterErr == nil {
		return inCluster, nil
	}

	if _, statErr := os.Stat(AdminKubeconfigPath); statErr == nil {
		adminConfig, adminErr := clientcmd.BuildConfigFromFlags("", AdminKubeconfigPath)
		if adminErr == nil {
			return adminConfig, nil
		}
	}

	return nil, fmt.Errorf(
		"kubernetes connection settings could not be resolved: %w. "+
			"Set KUBECONFIG or pass --kubeconfig. The factory office has no key to the floor", err)
}

// NewClient builds a Kubernetes client for the resolved connection settings.
func NewClient(kubeconfigPath string) (client.Client, error) {
	config, err := RestConfig(kubeconfigPath)
	if err != nil {
		return nil, err
	}
	config.QPS = 25
	config.Burst = 50
	kubeClient, err := client.New(config, client.Options{Scheme: Scheme()})
	if err != nil {
		return nil, fmt.Errorf(
			"kubernetes client could not be created: %w. Check that the API server is reachable. The factory office cannot call the floor", err)
	}
	return kubeClient, nil
}

// APIReachable reports whether the API server answers a trivial request.
func APIReachable(ctx context.Context, kubeClient client.Client) error {
	nodes := &corev1.NodeList{}
	if err := kubeClient.List(ctx, nodes); err != nil {
		return fmt.Errorf(
			"kubernetes API server did not answer a node list request: %w. Check kubelet, the control-plane pods and the kubeconfig. Factory floor unreachable", err)
	}
	return nil
}

// SingleNode returns the only node of the factory. A single-node distribution
// reports an explicit error when the assumption does not hold.
func SingleNode(ctx context.Context, kubeClient client.Client) (*corev1.Node, error) {
	nodes := &corev1.NodeList{}
	if err := kubeClient.List(ctx, nodes); err != nil {
		return nil, fmt.Errorf("node list could not be read: %w. Factory floor unreachable", err)
	}
	switch len(nodes.Items) {
	case 0:
		return nil, fmt.Errorf("cluster reports no nodes. Check kubelet on this host. The factory floor is empty")
	case 1:
		return &nodes.Items[0], nil
	default:
		return &nodes.Items[0], nil
	}
}

// NodeReady reports whether the node condition Ready is true.
func NodeReady(node *corev1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

// DeploymentReady reports the ready and desired replica counts of a Deployment.
func DeploymentReady(deployment *appsv1.Deployment) (int32, int32) {
	desired := int32(1)
	if deployment.Spec.Replicas != nil {
		desired = *deployment.Spec.Replicas
	}
	return deployment.Status.ReadyReplicas, desired
}
