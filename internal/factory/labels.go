// Package factory contains the deterministic build logic of the Brunernetes
// controllers: it turns a BrunerApp specification into the Kubernetes objects
// the factory owns, applies factory defaults, and evaluates factory policy.
//
// Everything in this package is a pure function of its inputs so that it can be
// unit tested without a Kubernetes API server.
package factory

import (
	brunerv1alpha1 "github.com/brunka09/brunernetes/api/v1alpha1"
)

// Well-known label and annotation keys used by the factory.
const (
	// LabelName is the standard application name label.
	LabelName = "app.kubernetes.io/name"
	// LabelInstance identifies the BrunerApp instance.
	LabelInstance = "app.kubernetes.io/instance"
	// LabelManagedBy marks objects owned by the Brunernetes controller.
	LabelManagedBy = "app.kubernetes.io/managed-by"
	// LabelComponent marks the role of the object.
	LabelComponent = "app.kubernetes.io/component"
	// LabelPartOf groups every object of the factory.
	LabelPartOf = "app.kubernetes.io/part-of"
	// LabelProfile records the effective profile on owned objects.
	LabelProfile = "bruner.dev/profile"
	// LabelShift records the effective shift on owned objects.
	LabelShift = "bruner.dev/shift"

	// AnnotationAppliedImage records the image the controller last applied.
	AnnotationAppliedImage = "bruner.dev/applied-image"
	// AnnotationQualityGate records the quality gate the object was built with.
	AnnotationQualityGate = "bruner.dev/quality-gate"
	// AnnotationLocalEndpoint records the loopback endpoint of the application.
	AnnotationLocalEndpoint = "bruner.dev/local-endpoint"

	// ManagerName is the value written to LabelManagedBy and used as the field
	// manager for server-side operations.
	ManagerName = "brunernetes"
	// PartOfName groups all factory objects.
	PartOfName = "brunernetes-factory"
	// ComponentApplication marks the workload objects of an application.
	ComponentApplication = "application"
)

// SelectorLabels returns the immutable label set used as the Deployment
// selector and the Service selector. These labels must never change for an
// existing application, because a Deployment selector is immutable.
func SelectorLabels(app *brunerv1alpha1.BrunerApp) map[string]string {
	return map[string]string{
		LabelName:     app.Name,
		LabelInstance: app.Name,
		LabelPartOf:   PartOfName,
	}
}

// ObjectLabels returns the full label set applied to owned objects. It always
// contains the selector labels.
func ObjectLabels(app *brunerv1alpha1.BrunerApp, mode brunerv1alpha1.BrunerMode) map[string]string {
	labels := SelectorLabels(app)
	out := make(map[string]string, len(labels)+4)
	for key, value := range labels {
		out[key] = value
	}
	out[LabelManagedBy] = ManagerName
	out[LabelComponent] = ComponentApplication
	if mode.Profile != "" {
		out[LabelProfile] = string(mode.Profile)
	}
	if mode.Shift != "" {
		out[LabelShift] = string(mode.Shift)
	}
	return out
}

// DeploymentName is the name of the Deployment owned by an application.
func DeploymentName(app *brunerv1alpha1.BrunerApp) string { return app.Name }

// ServiceName is the name of the Service owned by an application.
func ServiceName(app *brunerv1alpha1.BrunerApp) string { return app.Name }
