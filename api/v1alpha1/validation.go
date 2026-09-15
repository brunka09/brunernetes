package v1alpha1

import (
	"fmt"
	"strings"
)

// DefaultMode is the mode used when neither the application nor the factory
// policy sets a value.
func DefaultMode() BrunerMode {
	return BrunerMode{
		Profile:     ProfileGarage,
		Rollout:     RolloutSteady,
		QualityGate: QualityGatePractical,
		Shift:       ShiftDay,
	}
}

// MergeModes layers modes from lowest to highest precedence and fills every
// remaining empty field from DefaultMode. Nil entries are ignored, so callers
// can pass optional pointers directly.
func MergeModes(modes ...*BrunerMode) BrunerMode {
	merged := BrunerMode{}
	for _, mode := range modes {
		if mode == nil {
			continue
		}
		if mode.Profile != "" {
			merged.Profile = mode.Profile
		}
		if mode.Rollout != "" {
			merged.Rollout = mode.Rollout
		}
		if mode.QualityGate != "" {
			merged.QualityGate = mode.QualityGate
		}
		if mode.Shift != "" {
			merged.Shift = mode.Shift
		}
	}

	fallback := DefaultMode()
	if merged.Profile == "" {
		merged.Profile = fallback.Profile
	}
	if merged.Rollout == "" {
		merged.Rollout = fallback.Rollout
	}
	if merged.QualityGate == "" {
		merged.QualityGate = fallback.QualityGate
	}
	if merged.Shift == "" {
		merged.Shift = fallback.Shift
	}
	return merged
}

// ValidateMode reports whether every set field of the mode holds a known value.
func ValidateMode(mode BrunerMode) error {
	switch mode.Profile {
	case "", ProfileGarage, ProfileShowroom, ProfileStrict, ProfileFriday:
	default:
		return fmt.Errorf("unknown profile %q: allowed values are garage, showroom, strict and friday", mode.Profile)
	}
	switch mode.Rollout {
	case "", RolloutSteady:
	default:
		return fmt.Errorf("unknown rollout %q: the only supported rollout is steady", mode.Rollout)
	}
	switch mode.QualityGate {
	case "", QualityGatePractical, QualityGateStrict:
	default:
		return fmt.Errorf("unknown quality gate %q: allowed values are practical and strict", mode.QualityGate)
	}
	switch mode.Shift {
	case "", ShiftDay, ShiftNight:
	default:
		return fmt.Errorf("unknown shift %q: allowed values are day and night", mode.Shift)
	}
	return nil
}

// RequiresReadinessProbe reports whether the effective mode demands a readiness
// probe regardless of the factory policy flags.
func (m BrunerMode) RequiresReadinessProbe() bool {
	return m.Profile == ProfileStrict || m.QualityGate == QualityGateStrict
}

// RequiresResourceRequests reports whether the effective mode demands explicit
// CPU and memory requests.
func (m BrunerMode) RequiresResourceRequests() bool {
	return m.Profile == ProfileStrict || m.QualityGate == QualityGateStrict
}

// RequiresOverride reports whether the effective mode holds guarded changes,
// such as an image change, until a human approves them.
func (m BrunerMode) RequiresOverride() bool {
	return m.Profile == ProfileFriday
}

// ImageReference is a parsed container image reference.
type ImageReference struct {
	// Registry is the registry host, defaulting to docker.io.
	Registry string
	// Repository is the repository path without the registry host.
	Repository string
	// Tag is the image tag, empty when the reference is digest pinned.
	Tag string
	// Digest is the image digest, empty when the reference is tag based.
	Digest string
}

// String renders the parsed reference back into its canonical text form.
func (r ImageReference) String() string {
	name := r.Registry + "/" + r.Repository
	switch {
	case r.Digest != "":
		return name + "@" + r.Digest
	case r.Tag != "":
		return name + ":" + r.Tag
	default:
		return name
	}
}

// IsMutable reports whether the reference can silently change under the
// cluster. Digest pinned references are immutable; the ":latest" tag is not.
func (r ImageReference) IsMutable() bool {
	return r.Digest == "" && (r.Tag == "" || r.Tag == "latest")
}

// ParseImageReference parses a container image reference well enough for the
// policy checks Brunernetes performs. It does not contact a registry.
func ParseImageReference(image string) (ImageReference, error) {
	ref := ImageReference{}
	trimmed := strings.TrimSpace(image)
	if trimmed == "" {
		return ref, fmt.Errorf("image reference is empty")
	}
	if trimmed != image {
		return ref, fmt.Errorf("image reference %q contains leading or trailing whitespace", image)
	}
	if strings.ContainsAny(trimmed, " \t\n") {
		return ref, fmt.Errorf("image reference %q contains whitespace", image)
	}

	remainder := trimmed
	if at := strings.Index(remainder, "@"); at >= 0 {
		ref.Digest = remainder[at+1:]
		remainder = remainder[:at]
		if !strings.HasPrefix(ref.Digest, "sha256:") || len(ref.Digest) != len("sha256:")+64 {
			return ref, fmt.Errorf("image digest %q is not a valid sha256 digest", ref.Digest)
		}
	}

	// The registry host is the first path segment when it looks like a host:
	// it contains a dot or a colon, or it is exactly "localhost".
	name := remainder
	ref.Registry = "docker.io"
	if slash := strings.Index(name, "/"); slash >= 0 {
		candidate := name[:slash]
		if candidate == "localhost" || strings.ContainsAny(candidate, ".:") {
			ref.Registry = candidate
			name = name[slash+1:]
		}
	}

	if colon := strings.LastIndex(name, ":"); colon >= 0 {
		ref.Tag = name[colon+1:]
		name = name[:colon]
		if ref.Tag == "" {
			return ref, fmt.Errorf("image reference %q has an empty tag", image)
		}
	}

	if name == "" {
		return ref, fmt.Errorf("image reference %q has an empty repository", image)
	}
	for _, segment := range strings.Split(name, "/") {
		if segment == "" {
			return ref, fmt.Errorf("image reference %q has an empty repository segment", image)
		}
	}
	if ref.Registry == "docker.io" && !strings.Contains(name, "/") {
		name = "library/" + name
	}
	ref.Repository = name

	if ref.Digest == "" && ref.Tag == "" {
		ref.Tag = "latest"
	}
	return ref, nil
}

// RegistryAllowed reports whether the reference comes from one of the allowed
// registries. An empty allow list permits every registry.
func RegistryAllowed(ref ImageReference, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, entry := range allowed {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if entry == ref.Registry || strings.HasPrefix(ref.Registry+"/"+ref.Repository, strings.TrimSuffix(entry, "/")+"/") {
			return true
		}
	}
	return false
}

// IsLoopbackHostIP reports whether the address is inside the IPv4 loopback
// range. Brunernetes refuses to publish an application anywhere else.
func IsLoopbackHostIP(hostIP string) bool {
	return strings.HasPrefix(hostIP, "127.")
}
