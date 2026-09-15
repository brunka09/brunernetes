package v1alpha1

import "testing"

func TestParseImageReference(t *testing.T) {
	cases := []struct {
		name       string
		image      string
		wantErr    bool
		registry   string
		repository string
		tag        string
		digest     string
		mutable    bool
	}{
		{
			name:       "bare docker hub image gets the library namespace and the latest tag",
			image:      "nginx",
			registry:   "docker.io",
			repository: "library/nginx",
			tag:        "latest",
			mutable:    true,
		},
		{
			name:       "docker hub image with an explicit tag is immutable",
			image:      "nginx:1.27-alpine",
			registry:   "docker.io",
			repository: "library/nginx",
			tag:        "1.27-alpine",
		},
		{
			name:       "registry host is detected by its dot",
			image:      "registry.k8s.io/pause:3.10",
			registry:   "registry.k8s.io",
			repository: "pause",
			tag:        "3.10",
		},
		{
			name:       "registry host is detected by its port",
			image:      "localhost:5000/factory/showroom:v1",
			registry:   "localhost:5000",
			repository: "factory/showroom",
			tag:        "v1",
		},
		{
			name:       "bare localhost is a registry host",
			image:      "localhost/showroom:v1",
			registry:   "localhost",
			repository: "showroom",
			tag:        "v1",
		},
		{
			name:       "digest pinned reference is immutable and has no tag",
			image:      "ghcr.io/brunka09/showroom@sha256:" + repeat("a", 64),
			registry:   "ghcr.io",
			repository: "brunka09/showroom",
			digest:     "sha256:" + repeat("a", 64),
		},
		{
			name:       "nested repository path is preserved",
			image:      "ghcr.io/brunka09/brunernetes/showroom:v0.1.0",
			registry:   "ghcr.io",
			repository: "brunka09/brunernetes/showroom",
			tag:        "v0.1.0",
		},
		{name: "empty reference is rejected", image: "", wantErr: true},
		{name: "whitespace in the reference is rejected", image: "nginx :1.0", wantErr: true},
		{name: "leading whitespace is rejected", image: " nginx:1.0", wantErr: true},
		{name: "empty tag is rejected", image: "nginx:", wantErr: true},
		{name: "short digest is rejected", image: "nginx@sha256:abc", wantErr: true},
		{name: "non sha256 digest is rejected", image: "nginx@md5:" + repeat("a", 64), wantErr: true},
		{name: "empty repository segment is rejected", image: "ghcr.io//showroom:v1", wantErr: true},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			ref, err := ParseImageReference(testCase.image)
			if testCase.wantErr {
				if err == nil {
					t.Fatalf("expected an error for image %q, got %+v", testCase.image, ref)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for image %q: %v", testCase.image, err)
			}
			if ref.Registry != testCase.registry {
				t.Errorf("registry: got %q, want %q", ref.Registry, testCase.registry)
			}
			if ref.Repository != testCase.repository {
				t.Errorf("repository: got %q, want %q", ref.Repository, testCase.repository)
			}
			if ref.Tag != testCase.tag {
				t.Errorf("tag: got %q, want %q", ref.Tag, testCase.tag)
			}
			if ref.Digest != testCase.digest {
				t.Errorf("digest: got %q, want %q", ref.Digest, testCase.digest)
			}
			if testCase.mutable && !ref.IsMutable() {
				t.Errorf("image %q should be reported as mutable", testCase.image)
			}
			if !testCase.mutable && ref.IsMutable() {
				t.Errorf("image %q should be reported as immutable", testCase.image)
			}
		})
	}
}

func TestImageReferenceString(t *testing.T) {
	digest := "sha256:" + repeat("b", 64)
	cases := map[string]string{
		"nginx:1.27":            "docker.io/library/nginx:1.27",
		"ghcr.io/a/b@" + digest: "ghcr.io/a/b@" + digest,
	}
	for input, want := range cases {
		ref, err := ParseImageReference(input)
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", input, err)
		}
		if got := ref.String(); got != want {
			t.Errorf("String() for %q: got %q, want %q", input, got, want)
		}
	}
}

func TestRegistryAllowed(t *testing.T) {
	ref, err := ParseImageReference("ghcr.io/brunka09/showroom:v1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cases := []struct {
		name    string
		allowed []string
		want    bool
	}{
		{name: "empty list allows everything", allowed: nil, want: true},
		{name: "exact registry match is allowed", allowed: []string{"ghcr.io"}, want: true},
		{name: "repository prefix match is allowed", allowed: []string{"ghcr.io/brunka09"}, want: true},
		{name: "trailing slash is tolerated", allowed: []string{"ghcr.io/brunka09/"}, want: true},
		{name: "other registry is rejected", allowed: []string{"docker.io"}, want: false},
		{name: "other repository prefix is rejected", allowed: []string{"ghcr.io/someone-else"}, want: false},
		{name: "blank entries are ignored", allowed: []string{"  ", "ghcr.io"}, want: true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := RegistryAllowed(ref, testCase.allowed); got != testCase.want {
				t.Errorf("RegistryAllowed(%v): got %v, want %v", testCase.allowed, got, testCase.want)
			}
		})
	}
}

func TestMergeModes(t *testing.T) {
	factoryDefault := &BrunerMode{Profile: ProfileShowroom, Shift: ShiftNight}
	appOverride := &BrunerMode{Profile: ProfileStrict}

	merged := MergeModes(factoryDefault, appOverride)
	if merged.Profile != ProfileStrict {
		t.Errorf("application override must win: got %q", merged.Profile)
	}
	if merged.Shift != ShiftNight {
		t.Errorf("unset application fields must be inherited: got %q", merged.Shift)
	}
	if merged.Rollout != RolloutSteady {
		t.Errorf("rollout must fall back to the default: got %q", merged.Rollout)
	}
	if merged.QualityGate != QualityGatePractical {
		t.Errorf("quality gate must fall back to the default: got %q", merged.QualityGate)
	}

	empty := MergeModes(nil, nil)
	if empty != DefaultMode() {
		t.Errorf("merging nothing must produce the default mode: got %+v", empty)
	}
}

func TestValidateMode(t *testing.T) {
	if err := ValidateMode(DefaultMode()); err != nil {
		t.Errorf("the default mode must be valid: %v", err)
	}
	if err := ValidateMode(BrunerMode{}); err != nil {
		t.Errorf("an empty mode must be valid because every field is optional: %v", err)
	}
	invalid := []BrunerMode{
		{Profile: "monday"},
		{Rollout: "instant"},
		{QualityGate: "relaxed"},
		{Shift: "weekend"},
	}
	for _, mode := range invalid {
		if err := ValidateMode(mode); err == nil {
			t.Errorf("mode %+v must be rejected", mode)
		}
	}
}

func TestModeRequirements(t *testing.T) {
	strictProfile := BrunerMode{Profile: ProfileStrict, QualityGate: QualityGatePractical}
	if !strictProfile.RequiresReadinessProbe() || !strictProfile.RequiresResourceRequests() {
		t.Error("the strict profile must require a readiness probe and resource requests")
	}
	strictGate := BrunerMode{Profile: ProfileGarage, QualityGate: QualityGateStrict}
	if !strictGate.RequiresReadinessProbe() || !strictGate.RequiresResourceRequests() {
		t.Error("the strict quality gate must require a readiness probe and resource requests")
	}
	garage := BrunerMode{Profile: ProfileGarage, QualityGate: QualityGatePractical}
	if garage.RequiresReadinessProbe() || garage.RequiresResourceRequests() || garage.RequiresOverride() {
		t.Error("the garage profile must not require anything")
	}
	friday := BrunerMode{Profile: ProfileFriday}
	if !friday.RequiresOverride() {
		t.Error("the friday profile must require an override")
	}
}

func TestIsLoopbackHostIP(t *testing.T) {
	cases := map[string]bool{
		"127.0.0.1":   true,
		"127.10.0.5":  true,
		"0.0.0.0":     false,
		"192.168.1.5": false,
		"":            false,
	}
	for input, want := range cases {
		if got := IsLoopbackHostIP(input); got != want {
			t.Errorf("IsLoopbackHostIP(%q): got %v, want %v", input, got, want)
		}
	}
}

// repeat builds a string of n copies of a single character.
func repeat(char string, count int) string {
	out := ""
	for i := 0; i < count; i++ {
		out += char
	}
	return out
}
