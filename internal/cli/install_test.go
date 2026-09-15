package cli

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/util/yaml"

	brunerv1alpha1 "github.com/brunka09/brunernetes/api/v1alpha1"
)

func TestKubeadmConfigIsRenderedForASingleNode(t *testing.T) {
	rendered := KubeadmConfig(KubernetesVersion, PodCIDR, "10.0.0.5")

	mustContain(t, rendered, []string{
		"kind: InitConfiguration",
		"kind: ClusterConfiguration",
		"kind: KubeletConfiguration",
		"kubernetesVersion: " + KubernetesVersion,
		"podSubnet: " + PodCIDR,
		"serviceSubnet: " + ServiceCIDR,
		"advertiseAddress: 10.0.0.5",
		"cgroupDriver: systemd",
		ContainerdSocketURL,
	})

	// The advertise address is optional and must simply be absent when it is
	// not requested, so kubeadm can pick the default route address.
	withoutAddress := KubeadmConfig(KubernetesVersion, PodCIDR, "")
	if strings.Contains(withoutAddress, "advertiseAddress") {
		t.Errorf("no advertise address must be rendered when none is requested:\n%s", withoutAddress)
	}

	// The rendered configuration must be valid YAML in every document.
	for _, document := range strings.Split(rendered, "\n---\n") {
		if strings.TrimSpace(document) == "" {
			continue
		}
		parsed := map[string]any{}
		if err := yaml.Unmarshal([]byte(document), &parsed); err != nil {
			t.Fatalf("rendered kubeadm document is not valid YAML: %v\n%s", err, document)
		}
		if parsed["kind"] == nil {
			t.Errorf("rendered kubeadm document has no kind:\n%s", document)
		}
	}
}

func TestCNIConfigIsAMinimalBridgeNetwork(t *testing.T) {
	rendered := CNIConfig(PodCIDR)

	parsed := map[string]any{}
	if err := yaml.Unmarshal([]byte(rendered), &parsed); err != nil {
		t.Fatalf("the CNI configuration is not valid JSON: %v\n%s", err, rendered)
	}

	plugins, ok := parsed["plugins"].([]any)
	if !ok || len(plugins) == 0 {
		t.Fatalf("the CNI configuration must declare plugins:\n%s", rendered)
	}

	types := map[string]bool{}
	for _, plugin := range plugins {
		entry, ok := plugin.(map[string]any)
		if !ok {
			t.Fatalf("plugin entry is not an object: %v", plugin)
		}
		types[entry["type"].(string)] = true
	}
	// bridge plus host-local IPAM carries pod traffic; portmap is what makes
	// the loopback host port of an application work.
	for _, required := range []string{"bridge", "portmap"} {
		if !types[required] {
			t.Errorf("the %q plugin is required:\n%s", required, rendered)
		}
	}
	if !strings.Contains(rendered, PodCIDR) {
		t.Errorf("the pod CIDR must reach the IPAM configuration:\n%s", rendered)
	}
}

func TestFactoryManifestMatchesTheAPI(t *testing.T) {
	for _, preset := range ValidPresets() {
		rendered := FactoryManifest(preset, KubernetesVersion)

		var sawTogliatti, sawAvtoVAZ bool
		for _, document := range strings.Split(rendered, "\n---\n") {
			if strings.TrimSpace(document) == "" {
				continue
			}
			header := struct {
				APIVersion string `json:"apiVersion"`
				Kind       string `json:"kind"`
			}{}
			if err := yaml.Unmarshal([]byte(document), &header); err != nil {
				t.Fatalf("preset %q rendered invalid YAML: %v\n%s", preset, err, document)
			}
			if header.APIVersion != brunerv1alpha1.GroupVersion.String() {
				t.Errorf("preset %q rendered apiVersion %q, want %q",
					preset, header.APIVersion, brunerv1alpha1.GroupVersion.String())
			}

			switch header.Kind {
			case "Togliatti":
				sawTogliatti = true
				togliatti := &brunerv1alpha1.Togliatti{}
				if err := yaml.Unmarshal([]byte(document), togliatti); err != nil {
					t.Fatalf("preset %q rendered a Togliatti that does not match the API: %v", preset, err)
				}
				if togliatti.Name == "" {
					t.Errorf("preset %q rendered a Togliatti without a name", preset)
				}
			case "AvtoVAZ":
				sawAvtoVAZ = true
				avtovaz := &brunerv1alpha1.AvtoVAZ{}
				if err := yaml.Unmarshal([]byte(document), avtovaz); err != nil {
					t.Fatalf("preset %q rendered an AvtoVAZ that does not match the API: %v", preset, err)
				}
				if avtovaz.Spec.DefaultMode == nil {
					t.Fatalf("preset %q rendered an AvtoVAZ without a default mode", preset)
				}
				if err := brunerv1alpha1.ValidateMode(*avtovaz.Spec.DefaultMode); err != nil {
					t.Errorf("preset %q rendered an invalid default mode: %v", preset, err)
				}
				want := PresetMode(preset)
				if avtovaz.Spec.DefaultMode.Profile != want.Profile {
					t.Errorf("preset %q rendered profile %q, want %q",
						preset, avtovaz.Spec.DefaultMode.Profile, want.Profile)
				}
			default:
				t.Errorf("preset %q rendered an unexpected kind %q", preset, header.Kind)
			}
		}

		if !sawTogliatti || !sawAvtoVAZ {
			t.Errorf("preset %q must render both a Togliatti and an AvtoVAZ", preset)
		}
	}
}

func TestPresetModesAreValid(t *testing.T) {
	presets := ValidPresets()
	if len(presets) == 0 {
		t.Fatal("at least one installation preset must exist")
	}
	seen := map[brunerv1alpha1.Profile]bool{}
	for _, preset := range presets {
		mode := PresetMode(preset)
		if err := brunerv1alpha1.ValidateMode(mode); err != nil {
			t.Errorf("preset %q produces an invalid mode: %v", preset, err)
		}
		seen[mode.Profile] = true
	}
	if len(seen) < 2 {
		t.Error("the presets must differ from each other, otherwise they are decoration")
	}
}

func TestDryRunnerChangesNothing(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "nested", "kubeadm.yaml")
	printer := quietPrinter()

	runner := &DryRunner{Printer: printer}
	if _, err := runner.Run(context.Background(), "kubeadm", "init"); err != nil {
		t.Fatalf("a dry run must not fail: %v", err)
	}
	if err := runner.WriteFile(target, "content", 0o644); err != nil {
		t.Fatalf("a dry run write must not fail: %v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Errorf("a dry run must not create %s", target)
	}
	if err := runner.RemoveAll(directory); err != nil {
		t.Fatalf("a dry run removal must not fail: %v", err)
	}
	if _, err := os.Stat(directory); err != nil {
		t.Errorf("a dry run must not delete %s: %v", directory, err)
	}
}

func TestInstallDryRunTouchesNothingAndRunsEveryStep(t *testing.T) {
	printer := quietPrinter()
	recorder := &recordingRunner{}
	installer := &Installer{
		Printer: printer,
		Runner:  recorder,
		Options: InstallOptions{
			Preset:            "garage",
			DryRun:            true,
			SkipPreflight:     true,
			KubernetesVersion: KubernetesVersion,
			PodCIDR:           PodCIDR,
			ChartPath:         t.TempDir(),
		},
	}

	if err := installer.Install(context.Background()); err != nil {
		t.Fatalf("a dry run installation must not fail: %v", err)
	}
	if len(recorder.commands) == 0 {
		t.Fatal("the installer must report the commands it would run")
	}

	joined := strings.Join(recorder.commands, "\n")
	for _, expected := range []string{"kubeadm", "systemctl", "kubectl"} {
		if !strings.Contains(joined, expected) {
			t.Errorf("the installation must involve %q:\n%s", expected, joined)
		}
	}
	if len(recorder.files) == 0 {
		t.Error("the installer must report the files it would write")
	}
	for path := range recorder.files {
		if !strings.HasPrefix(path, "/") {
			t.Errorf("the installer must write absolute paths: %q", path)
		}
	}
}

// quietPrinter returns a printer that discards its output, so the tests stay
// readable.
func quietPrinter() *Printer {
	return &Printer{Out: io.Discard, Err: io.Discard}
}

// recordingRunner captures every operation instead of performing it.
type recordingRunner struct {
	commands []string
	files    map[string]string
	removed  []string
}

func (r *recordingRunner) Run(_ context.Context, name string, args ...string) (string, error) {
	r.commands = append(r.commands, strings.Join(append([]string{name}, args...), " "))
	return "", nil
}

func (r *recordingRunner) RunWithInput(_ context.Context, _ string, name string, args ...string) (string, error) {
	r.commands = append(r.commands, strings.Join(append([]string{name}, args...), " "))
	return "", nil
}

func (r *recordingRunner) WriteFile(path string, content string, _ os.FileMode) error {
	if r.files == nil {
		r.files = map[string]string{}
	}
	r.files[path] = content
	return nil
}

func (r *recordingRunner) RemoveAll(path string) error {
	r.removed = append(r.removed, path)
	return nil
}

// mustContain fails the test when any fragment is missing from the text.
func mustContain(t *testing.T, text string, fragments []string) {
	t.Helper()
	for _, fragment := range fragments {
		if !strings.Contains(text, fragment) {
			t.Errorf("rendered output is missing %q:\n%s", fragment, text)
		}
	}
}
