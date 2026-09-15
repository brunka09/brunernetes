package factory

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	brunerv1alpha1 "github.com/brunka09/brunernetes/api/v1alpha1"
)

// policyApp returns an application that satisfies every strict requirement,
// so each test only has to break the one thing it is about.
func policyApp(mutators ...func(*brunerv1alpha1.BrunerApp)) *brunerv1alpha1.BrunerApp {
	app := &brunerv1alpha1.BrunerApp{
		ObjectMeta: metav1.ObjectMeta{Name: "showroom", Namespace: "brunernetes-demo"},
		Spec: brunerv1alpha1.BrunerAppSpec{
			Image: "ghcr.io/brunka09/showroom:v0.1.0",
			ReadinessProbe: &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/healthz"}},
			},
			Resources: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("50m"),
					corev1.ResourceMemory: resource.MustParse("64Mi"),
				},
			},
		},
	}
	for _, mutate := range mutators {
		mutate(app)
	}
	return app
}

func TestDefaultPolicyIsSingleApplicationAndPrivate(t *testing.T) {
	policy := DefaultPolicy()
	if policy.MaxApplications != 1 {
		t.Errorf("the MVP factory runs one application: got %d", policy.MaxApplications)
	}
	if policy.AllowPublicExposure != nil && *policy.AllowPublicExposure {
		t.Error("public exposure must never be allowed by default")
	}
	if policy.LatestTagPolicy != brunerv1alpha1.LatestTagWarn {
		t.Errorf("the default latest tag policy must warn: got %q", policy.LatestTagPolicy)
	}
}

func TestNormalizePolicyFillsUnsetFields(t *testing.T) {
	normalized := NormalizePolicy(nil)
	if normalized.MaxApplications != DefaultPolicy().MaxApplications {
		t.Error("a nil policy must normalize to the defaults")
	}

	custom := NormalizePolicy(&brunerv1alpha1.FactoryPolicy{MaxApplications: 5})
	if custom.MaxApplications != 5 {
		t.Errorf("an explicit limit must be preserved: got %d", custom.MaxApplications)
	}
	if custom.LatestTagPolicy != DefaultPolicy().LatestTagPolicy {
		t.Error("unset fields must be filled from the defaults")
	}
}

func TestEvaluateAppAcceptsACompliantApplication(t *testing.T) {
	result := EvaluateApp(policyApp(), DefaultPolicy(), brunerv1alpha1.DefaultMode(), "")
	if result.Blocked() {
		t.Fatalf("a compliant application must not be blocked: %s", result.Summary())
	}
	if len(result.Warnings) != 0 {
		t.Errorf("a compliant application must not raise warnings: %s", result.Summary())
	}
}

func TestEvaluateAppRejectsAnInvalidImageReference(t *testing.T) {
	app := policyApp(func(a *brunerv1alpha1.BrunerApp) { a.Spec.Image = "ghcr.io/broken:" })
	result := EvaluateApp(app, DefaultPolicy(), brunerv1alpha1.DefaultMode(), "")
	if !result.Blocked() {
		t.Fatal("an unparseable image reference must block the rollout")
	}
	violation, _ := result.FirstViolation()
	if !strings.Contains(strings.ToLower(violation.Message), "image") {
		t.Errorf("the message must name the image: %q", violation.Message)
	}
}

func TestEvaluateAppHandlesTheLatestTag(t *testing.T) {
	app := policyApp(func(a *brunerv1alpha1.BrunerApp) { a.Spec.Image = "ghcr.io/brunka09/showroom:latest" })

	warnPolicy := DefaultPolicy()
	warnResult := EvaluateApp(app, warnPolicy, brunerv1alpha1.DefaultMode(), "")
	if warnResult.Blocked() {
		t.Error("the warn policy must not block a mutable tag")
	}
	if len(warnResult.Warnings) == 0 {
		t.Error("the warn policy must raise a warning for a mutable tag")
	}

	rejectPolicy := DefaultPolicy()
	rejectPolicy.LatestTagPolicy = brunerv1alpha1.LatestTagReject
	if !EvaluateApp(app, rejectPolicy, brunerv1alpha1.DefaultMode(), "").Blocked() {
		t.Error("the reject policy must block a mutable tag")
	}

	ignorePolicy := DefaultPolicy()
	ignorePolicy.LatestTagPolicy = brunerv1alpha1.LatestTagIgnore
	ignoreResult := EvaluateApp(app, ignorePolicy, brunerv1alpha1.DefaultMode(), "")
	if ignoreResult.Blocked() || len(ignoreResult.Warnings) != 0 {
		t.Errorf("the ignore policy must stay silent: %s", ignoreResult.Summary())
	}
}

func TestEvaluateAppEnforcesTheAllowedRegistries(t *testing.T) {
	policy := DefaultPolicy()
	policy.AllowedRegistries = []string{"ghcr.io/brunka09"}

	if EvaluateApp(policyApp(), policy, brunerv1alpha1.DefaultMode(), "").Blocked() {
		t.Error("an image from an allowed registry must pass")
	}

	foreign := policyApp(func(a *brunerv1alpha1.BrunerApp) { a.Spec.Image = "docker.io/library/nginx:1.27" })
	if !EvaluateApp(foreign, policy, brunerv1alpha1.DefaultMode(), "").Blocked() {
		t.Error("an image from a registry outside the allow list must be blocked")
	}
}

func TestEvaluateAppEnforcesStrictRequirements(t *testing.T) {
	strict := brunerv1alpha1.BrunerMode{QualityGate: brunerv1alpha1.QualityGateStrict}

	noProbe := policyApp(func(a *brunerv1alpha1.BrunerApp) { a.Spec.ReadinessProbe = nil })
	if !EvaluateApp(noProbe, DefaultPolicy(), strict, "").Blocked() {
		t.Error("the strict quality gate must require a readiness probe")
	}
	if EvaluateApp(noProbe, DefaultPolicy(), brunerv1alpha1.DefaultMode(), "").Blocked() {
		t.Error("the practical quality gate must accept a missing readiness probe")
	}

	noResources := policyApp(func(a *brunerv1alpha1.BrunerApp) { a.Spec.Resources = nil })
	if !EvaluateApp(noResources, DefaultPolicy(), strict, "").Blocked() {
		t.Error("the strict quality gate must require resource requests")
	}
}

func TestEvaluateAppEnforcesTheFridayGuard(t *testing.T) {
	friday := brunerv1alpha1.BrunerMode{Profile: brunerv1alpha1.ProfileFriday}
	app := policyApp()

	if EvaluateApp(app, DefaultPolicy(), friday, "").Blocked() {
		t.Error("a first rollout must not be blocked by the friday guard")
	}

	changed := EvaluateApp(app, DefaultPolicy(), friday, "ghcr.io/brunka09/showroom:v0.0.9")
	if !changed.Blocked() {
		t.Fatal("an image change must be blocked by the friday guard until it is approved")
	}
	violation, _ := changed.FirstViolation()
	if !strings.Contains(strings.ToLower(violation.Message), "override") {
		t.Errorf("the message must explain how to approve the change: %q", violation.Message)
	}

	approved := policyApp(func(a *brunerv1alpha1.BrunerApp) {
		a.Spec.Override = &brunerv1alpha1.AppOverride{
			Approved: true,
			Image:    a.Spec.Image,
			Reason:   "hotfix approved by the on-call engineer",
		}
	})
	if EvaluateApp(approved, DefaultPolicy(), friday, "ghcr.io/brunka09/showroom:v0.0.9").Blocked() {
		t.Error("a matching approved override must release the friday guard")
	}

	mismatched := policyApp(func(a *brunerv1alpha1.BrunerApp) {
		a.Spec.Override = &brunerv1alpha1.AppOverride{Approved: true, Image: "ghcr.io/brunka09/other:v1"}
	})
	if !EvaluateApp(mismatched, DefaultPolicy(), friday, "ghcr.io/brunka09/showroom:v0.0.9").Blocked() {
		t.Error("an override for a different image must not release the guard")
	}
}

func TestEvaluateAppNormalisesPublicExposure(t *testing.T) {
	app := policyApp(func(a *brunerv1alpha1.BrunerApp) {
		a.Spec.LocalExposure = &brunerv1alpha1.LocalExposure{Enabled: ptr.To(true), HostIP: "0.0.0.0"}
	})
	result := EvaluateApp(app, DefaultPolicy(), brunerv1alpha1.DefaultMode(), "")
	if result.Blocked() {
		t.Fatalf("a non-loopback host IP is normalised rather than blocked: %s", result.Summary())
	}
	if len(result.Warnings) == 0 {
		t.Fatal("a non-loopback host IP must raise a warning")
	}
	if !strings.Contains(result.Warnings[0].Message, "127.0.0.1") {
		t.Errorf("the warning must name the address that is used instead: %q", result.Warnings[0].Message)
	}
	// The build layer must actually apply the normalisation the warning promises.
	if got := ResolvedExposure(app).HostIP; got != DefaultHostIP {
		t.Errorf("the built object must publish on %s: got %s", DefaultHostIP, got)
	}
}

func TestEvaluateCapacity(t *testing.T) {
	policy := DefaultPolicy()
	if _, exceeded := EvaluateCapacity(1, policy); exceeded {
		t.Error("one application must fit within the default limit")
	}
	finding, exceeded := EvaluateCapacity(2, policy)
	if !exceeded {
		t.Fatal("a second application must exceed the default limit")
	}
	if !strings.Contains(finding.Message, "1") {
		t.Errorf("the message must state the limit: %q", finding.Message)
	}
}

func TestPolicyResultSummaryListsViolationsFirst(t *testing.T) {
	result := PolicyResult{
		Warnings:   []Finding{{Reason: "W", Message: "warning text"}},
		Violations: []Finding{{Reason: "V", Message: "violation text"}},
	}
	summary := result.Summary()
	if strings.Index(summary, "violation text") > strings.Index(summary, "warning text") {
		t.Errorf("violations must be reported before warnings: %q", summary)
	}
	empty := PolicyResult{}
	if empty.Summary() == "" {
		t.Error("an empty result must still produce a readable summary")
	}
}
