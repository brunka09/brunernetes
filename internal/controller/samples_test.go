package controller

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TestShippedManifestsAreAccepted applies every manifest that ships with the
// repository to a real API server. The generated CRD schemas are the only
// validation layer in this design, so a sample with a misspelled field would
// otherwise only fail on a user's machine.
func TestShippedManifestsAreAccepted(t *testing.T) {
	config, stop := startEnvironment(t)
	defer stop()

	manifestScheme := testScheme(t)
	if err := scheme.AddToScheme(manifestScheme); err != nil {
		t.Fatalf("core types could not be registered: %v", err)
	}
	kubeClient, err := client.New(config, client.Options{Scheme: manifestScheme})
	if err != nil {
		t.Fatalf("the test client could not be created: %v", err)
	}
	decoder := serializer.NewCodecFactory(manifestScheme).UniversalDeserializer()
	ctx := context.Background()

	directories := []string{
		filepath.Join("..", "..", "config", "samples"),
		filepath.Join("..", "..", "demo", "manifests"),
	}

	applied := 0
	for _, directory := range directories {
		entries, err := os.ReadDir(directory)
		if err != nil {
			t.Fatalf("%s could not be read: %v", directory, err)
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasSuffix(name, ".yaml") {
				continue
			}
			// kustomization.yaml describes the other files; it is not a
			// resource the API server knows about.
			if name == "kustomization.yaml" {
				continue
			}

			path := filepath.Join(directory, name)
			t.Run(filepath.Join(filepath.Base(directory), name), func(t *testing.T) {
				raw, err := os.ReadFile(path)
				if err != nil {
					t.Fatalf("the manifest could not be read: %v", err)
				}
				for index, document := range splitDocuments(string(raw)) {
					object, _, err := decoder.Decode([]byte(document), nil, nil)
					if err != nil {
						t.Fatalf("document %d does not decode into a known kind: %v", index+1, err)
					}
					typed, ok := object.(client.Object)
					if !ok {
						t.Fatalf("document %d is not a Kubernetes object: %T", index+1, object)
					}
					// The manifests are applied in directory order, so the
					// namespace of an object may not exist yet. kubectl would
					// report the same, which is why demo/manifests ships a
					// namespace file, but the test must not depend on the order.
					ensureNamespace(t, ctx, kubeClient, typed.GetNamespace())
					if err := kubeClient.Create(ctx, typed); err != nil && !apierrors.IsAlreadyExists(err) {
						t.Fatalf("the API server rejected document %d (%s): %v",
							index+1, typed.GetObjectKind().GroupVersionKind().Kind, err)
					}
					applied++
				}
			})
		}
	}

	if applied == 0 {
		t.Fatal("no manifest was applied, so the shipped examples are not covered")
	}
	t.Logf("%d shipped manifests were accepted by the API server.", applied)
}

// ensureNamespace creates a namespace when it is missing.
func ensureNamespace(t *testing.T, ctx context.Context, kubeClient client.Client, name string) {
	t.Helper()
	if name == "" {
		return
	}
	namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
	if err := kubeClient.Create(ctx, namespace); err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("the namespace %s could not be created: %v", name, err)
	}
}

// splitDocuments cuts a multi document YAML file into its documents and drops
// the empty and comment only ones.
func splitDocuments(content string) []string {
	var documents []string
	for _, part := range strings.Split(content, "\n---") {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		meaningful := false
		for _, line := range strings.Split(trimmed, "\n") {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "#") {
				meaningful = true
				break
			}
		}
		if meaningful {
			documents = append(documents, trimmed)
		}
	}
	return documents
}
