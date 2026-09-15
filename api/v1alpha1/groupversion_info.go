// Package v1alpha1 contains the Brunernetes factory API definitions.
//
// The API models a single-node Kubernetes "factory": Togliatti describes the
// factory environment, AvtoVAZ describes factory-wide policy, BrunerApp
// describes the single locally exposed application, and Konveer describes a
// lightweight delivery workflow for an already-built container image.
//
// +kubebuilder:object:generate=true
// +groupName=bruner.dev
package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

var (
	// GroupVersion is the group version used to register these objects.
	GroupVersion = schema.GroupVersion{Group: "bruner.dev", Version: "v1alpha1"}

	// SchemeBuilder registers the Brunernetes types with a runtime scheme.
	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}

	// AddToScheme adds the Brunernetes types to a runtime scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)
