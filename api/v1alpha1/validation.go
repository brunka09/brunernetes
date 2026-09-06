// Copyright 2026 Julia Bruner.
// Licensed under the MIT License.

package v1alpha1

import (
	"fmt"
	"regexp"
	"strings"
)

var imageReferencePattern = regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)*(?::[0-9]+)?(?:/[a-z0-9]+(?:[._-][a-z0-9]+)*)+(?:(?::[A-Za-z0-9_][A-Za-z0-9_.-]{0,127})|(?:@sha256:[a-f0-9]{64}))?$`)

// IsValidImageReference reports whether reference satisfies the MVP OCI-style image reference policy.
func IsValidImageReference(reference string) bool {
	return imageReferencePattern.MatchString(reference)
}

// HasLatestTag reports whether reference explicitly selects the latest tag or omits both tag and digest.
func HasLatestTag(reference string) bool {
	if strings.Contains(reference, "@") {
		return false
	}
	lastSlash := strings.LastIndex(reference, "/")
	lastColon := strings.LastIndex(reference, ":")
	if lastColon <= lastSlash {
		return true
	}
	return reference[lastColon+1:] == "latest"
}

// IsValidLocalhostPort reports whether port can be used by the local gateway without privileged binding.
func IsValidLocalhostPort(port int32) bool {
	return port >= 1024 && port <= 65535
}

// IsValidGatewayPortRange reports whether a local gateway port range is ordered and valid.
func IsValidGatewayPortRange(min, max int32) bool {
	return IsValidLocalhostPort(min) && IsValidLocalhostPort(max) && min <= max
}

// IsFridayOverrideEnabled reports whether annotations contain the configured explicit approval value.
func IsFridayOverrideEnabled(annotations map[string]string, key string) bool {
	return key != "" && annotations != nil && annotations[key] == "approved"
}

// ValidateBrunerMode validates a complete BrunerMode value for callers outside API server validation.
func ValidateBrunerMode(mode BrunerMode) error {
	if mode.Profile != BrunerProfileGarage && mode.Profile != BrunerProfileShowroom && mode.Profile != BrunerProfileStrict && mode.Profile != BrunerProfileFriday {
		return fmt.Errorf("profile must be garage, showroom, strict, or friday")
	}
	if mode.Rollout != BrunerRolloutSteady {
		return fmt.Errorf("rollout must be steady")
	}
	if mode.QualityGate != BrunerQualityGatePractical && mode.QualityGate != BrunerQualityGateStrict {
		return fmt.Errorf("qualityGate must be practical or strict")
	}
	if mode.Shift != BrunerShiftDay && mode.Shift != BrunerShiftNight {
		return fmt.Errorf("shift must be day or night")
	}
	return nil
}
