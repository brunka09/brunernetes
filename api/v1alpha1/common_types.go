// Copyright 2026 Julia Bruner.
// Licensed under the MIT License.

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BrunerProfile selects the local developer experience profile.
// +kubebuilder:validation:Enum=garage;showroom;strict;friday
type BrunerProfile string

const (
	BrunerProfileGarage   BrunerProfile = "garage"
	BrunerProfileShowroom BrunerProfile = "showroom"
	BrunerProfileStrict   BrunerProfile = "strict"
	BrunerProfileFriday   BrunerProfile = "friday"
)

// BrunerRollout selects the supported rollout behavior.
// +kubebuilder:validation:Enum=steady
type BrunerRollout string

const BrunerRolloutSteady BrunerRollout = "steady"

// BrunerQualityGate selects quality enforcement behavior.
// +kubebuilder:validation:Enum=practical;strict
type BrunerQualityGate string

const (
	BrunerQualityGatePractical BrunerQualityGate = "practical"
	BrunerQualityGateStrict    BrunerQualityGate = "strict"
)

// BrunerShift identifies the active factory shift.
// +kubebuilder:validation:Enum=day;night
type BrunerShift string

const (
	BrunerShiftDay   BrunerShift = "day"
	BrunerShiftNight BrunerShift = "night"
)

// BrunerMode defines the profile and quality behavior used by a delivery or application.
type BrunerMode struct {
	// Profile selects garage, showroom, strict, or friday behavior.
	// +kubebuilder:default:=garage
	Profile BrunerProfile `json:"profile"`

	// Rollout selects the rollout behavior. Only steady is supported in v0.1.
	// +kubebuilder:default:=steady
	Rollout BrunerRollout `json:"rollout"`

	// QualityGate selects practical or strict quality checks.
	// +kubebuilder:default:=practical
	QualityGate BrunerQualityGate `json:"qualityGate"`

	// Shift identifies the current factory shift.
	// +kubebuilder:default:=day
	Shift BrunerShift `json:"shift"`
}

// ResourceRequirements is an alias retained for concise API field declarations.
type ResourceRequirements = corev1.ResourceRequirements

// Conditions is a list of standard Kubernetes status conditions.
// +listType=map
// +listMapKey=type
type Conditions []metav1.Condition
