package factory

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"

	brunerv1alpha1 "github.com/brunka09/brunernetes/api/v1alpha1"
)

// Finding is a single policy observation. Message always states the technical
// condition first and adds themed wording afterwards.
type Finding struct {
	// Reason is the machine readable reason, reused as the condition reason and
	// the event reason.
	Reason string
	// Message is the human readable, English-only explanation.
	Message string
}

// PolicyResult is the outcome of evaluating factory policy against an
// application. Violations block the rollout, warnings do not.
type PolicyResult struct {
	Violations []Finding
	Warnings   []Finding
}

// Blocked reports whether the rollout must be held back.
func (r PolicyResult) Blocked() bool { return len(r.Violations) > 0 }

// Summary renders the findings as a single line, violations first.
func (r PolicyResult) Summary() string {
	if len(r.Violations) == 0 && len(r.Warnings) == 0 {
		return "All factory policy checks passed. Incoming inspection found nothing to report."
	}
	messages := make([]string, 0, len(r.Violations)+len(r.Warnings))
	for _, finding := range r.Violations {
		messages = append(messages, finding.Message)
	}
	for _, finding := range r.Warnings {
		messages = append(messages, finding.Message)
	}
	return strings.Join(messages, " ")
}

// FirstViolation returns the first blocking finding, if any.
func (r PolicyResult) FirstViolation() (Finding, bool) {
	if len(r.Violations) == 0 {
		return Finding{}, false
	}
	return r.Violations[0], true
}

// DefaultPolicy returns the factory policy applied when no AvtoVAZ resource
// exists in the cluster.
func DefaultPolicy() brunerv1alpha1.FactoryPolicy {
	requireReadiness := false
	requireRequests := false
	allowPublic := false
	return brunerv1alpha1.FactoryPolicy{
		RequireReadinessProbe:   &requireReadiness,
		RequireResourceRequests: &requireRequests,
		LatestTagPolicy:         brunerv1alpha1.LatestTagWarn,
		MaxApplications:         1,
		AllowPublicExposure:     &allowPublic,
	}
}

// NormalizePolicy fills unset policy fields with the factory defaults.
func NormalizePolicy(policy *brunerv1alpha1.FactoryPolicy) brunerv1alpha1.FactoryPolicy {
	normalized := DefaultPolicy()
	if policy == nil {
		return normalized
	}
	if policy.RequireReadinessProbe != nil {
		value := *policy.RequireReadinessProbe
		normalized.RequireReadinessProbe = &value
	}
	if policy.RequireResourceRequests != nil {
		value := *policy.RequireResourceRequests
		normalized.RequireResourceRequests = &value
	}
	if policy.LatestTagPolicy != "" {
		normalized.LatestTagPolicy = policy.LatestTagPolicy
	}
	if policy.MaxApplications > 0 {
		normalized.MaxApplications = policy.MaxApplications
	}
	if len(policy.AllowedRegistries) > 0 {
		normalized.AllowedRegistries = append([]string(nil), policy.AllowedRegistries...)
	}
	// Public exposure stays disabled in every configuration. The field exists
	// to make the localhost-only guarantee explicit and auditable.
	return normalized
}

// hasResourceRequests reports whether both CPU and memory requests are set
// explicitly in the specification.
func hasResourceRequests(resources *corev1.ResourceRequirements) bool {
	if resources == nil {
		return false
	}
	_, cpu := resources.Requests[corev1.ResourceCPU]
	_, memory := resources.Requests[corev1.ResourceMemory]
	return cpu && memory
}

// overrideApproves reports whether the override explicitly approves the image.
func overrideApproves(override *brunerv1alpha1.AppOverride, image string) bool {
	if override == nil || !override.Approved {
		return false
	}
	if override.Image == "" {
		return true
	}
	return override.Image == image
}

// EvaluateApp checks an application against factory policy and the effective
// mode. previousImage is the image the controller last applied; it is empty for
// a first rollout, where the friday guard does not apply.
func EvaluateApp(
	app *brunerv1alpha1.BrunerApp,
	policy brunerv1alpha1.FactoryPolicy,
	mode brunerv1alpha1.BrunerMode,
	previousImage string,
) PolicyResult {
	result := PolicyResult{}
	strict := mode.QualityGate == brunerv1alpha1.QualityGateStrict

	// 1. The image reference must be parsable before anything else is checked.
	ref, err := brunerv1alpha1.ParseImageReference(app.Spec.Image)
	if err != nil {
		result.Violations = append(result.Violations, Finding{
			Reason: brunerv1alpha1.ReasonValidationFailed,
			Message: fmt.Sprintf(
				"Container image reference is invalid: %v. Correct spec.image before the rollout can continue. Parts order rejected at the gate.",
				err),
		})
		return result
	}

	// 2. Mutable tags make a rollout unreproducible.
	if ref.IsMutable() {
		message := fmt.Sprintf(
			"Container image %q uses a mutable tag. Pin an immutable tag or a sha256 digest so the rollout is reproducible. Parts arrived without a batch number.",
			app.Spec.Image)
		switch policy.LatestTagPolicy {
		case brunerv1alpha1.LatestTagReject:
			result.Violations = append(result.Violations, Finding{
				Reason:  brunerv1alpha1.ReasonPolicyViolation,
				Message: message,
			})
		case brunerv1alpha1.LatestTagIgnore:
			// Accepted without comment by explicit policy.
		default:
			result.Warnings = append(result.Warnings, Finding{
				Reason:  brunerv1alpha1.ReasonPolicyDeviation,
				Message: message,
			})
		}
	}

	// 3. The registry allow list, when configured, is always blocking.
	if !brunerv1alpha1.RegistryAllowed(ref, policy.AllowedRegistries) {
		result.Violations = append(result.Violations, Finding{
			Reason: brunerv1alpha1.ReasonPolicyViolation,
			Message: fmt.Sprintf(
				"Container registry %q is not in the allowed registry list %v. Use an approved registry. Supplier is not on the approved list.",
				ref.Registry, policy.AllowedRegistries),
		})
	}

	// 4. Readiness probe requirement.
	readinessRequired := mode.RequiresReadinessProbe() ||
		(policy.RequireReadinessProbe != nil && *policy.RequireReadinessProbe)
	if readinessRequired && app.Spec.ReadinessProbe == nil {
		message := "Readiness probe is not defined in the specification. Set spec.readinessProbe so readiness is measured explicitly. Final inspection has no checklist."
		finding := Finding{Reason: brunerv1alpha1.ReasonPolicyViolation, Message: message}
		if strict || mode.Profile == brunerv1alpha1.ProfileStrict {
			result.Violations = append(result.Violations, finding)
		} else {
			finding.Reason = brunerv1alpha1.ReasonPolicyDeviation
			result.Warnings = append(result.Warnings, finding)
		}
	}

	// 5. Resource request requirement.
	requestsRequired := mode.RequiresResourceRequests() ||
		(policy.RequireResourceRequests != nil && *policy.RequireResourceRequests)
	if requestsRequired && !hasResourceRequests(app.Spec.Resources) {
		message := "CPU and memory requests are not set in the specification. Set spec.resources.requests so the scheduler can plan capacity. Line capacity cannot be planned without part weights."
		finding := Finding{Reason: brunerv1alpha1.ReasonPolicyViolation, Message: message}
		if strict || mode.Profile == brunerv1alpha1.ProfileStrict {
			result.Violations = append(result.Violations, finding)
		} else {
			finding.Reason = brunerv1alpha1.ReasonPolicyDeviation
			result.Warnings = append(result.Warnings, finding)
		}
	}

	// 6. The friday profile holds guarded changes until a human approves them.
	if mode.RequiresOverride() && previousImage != "" && previousImage != app.Spec.Image {
		if !overrideApproves(app.Spec.Override, app.Spec.Image) {
			result.Violations = append(result.Violations, Finding{
				Reason: brunerv1alpha1.ReasonOverrideRequired,
				Message: fmt.Sprintf(
					"Image change from %q to %q is held back by the friday profile. Set spec.override.approved to true with a matching spec.override.image to continue. No unsigned changes go to the line before the weekend.",
					previousImage, app.Spec.Image),
			})
		}
	}

	// 7. A non-loopback host IP is normalised, never honoured.
	if app.Spec.LocalExposure != nil && app.Spec.LocalExposure.HostIP != "" &&
		!brunerv1alpha1.IsLoopbackHostIP(app.Spec.LocalExposure.HostIP) {
		result.Warnings = append(result.Warnings, Finding{
			Reason: brunerv1alpha1.ReasonPolicyDeviation,
			Message: fmt.Sprintf(
				"Host IP %q is not a loopback address and was replaced with %s. Brunernetes publishes applications on localhost only. The showroom has no street entrance.",
				app.Spec.LocalExposure.HostIP, DefaultHostIP),
		})
	}

	return result
}

// EvaluateCapacity checks the number of applications against the factory limit.
// observed is the total number of BrunerApp resources in the cluster.
func EvaluateCapacity(observed int32, policy brunerv1alpha1.FactoryPolicy) (Finding, bool) {
	if observed <= policy.MaxApplications {
		return Finding{}, false
	}
	return Finding{
		Reason: brunerv1alpha1.ReasonCapacityExceeded,
		Message: fmt.Sprintf(
			"Cluster holds %d BrunerApp resources but the factory policy allows %d. Delete the extra applications. Only one product line fits on this floor.",
			observed, policy.MaxApplications),
	}, true
}
