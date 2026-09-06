// Copyright 2026 Julia Bruner.
// Licensed under the MIT License.

// Package v1alpha1 contains API Schema definitions for the bruner.dev v1alpha1 API group.
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	GroupVersion = schema.GroupVersion{Group: "bruner.dev", Version: "v1alpha1"}

	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)

	AddToScheme = SchemeBuilder.AddToScheme
)

func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(GroupVersion,
		&Togliatti{},
		&TogliattiList{},
		&AvtoVAZ{},
		&AvtoVAZList{},
		&BrunerApp{},
		&BrunerAppList{},
		&Konveer{},
		&KonveerList{},
	)
	metav1.AddToGroupVersion(scheme, GroupVersion)
	return nil
}
