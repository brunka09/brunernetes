# Repository structure

Annotated layout of the Brunernetes repository. Verified against the working
tree; directories that are planned but still empty are marked explicitly rather
than described as if they were populated.

Go module: `github.com/brunka09/brunernetes`, Go 1.24.6,
`k8s.io/*` v0.32.3, `sigs.k8s.io/controller-runtime` v0.20.4,
`github.com/spf13/cobra` v1.8.1.

```
.
├── api/
│   └── v1alpha1/                       API types. The source of truth for every CRD.
│       ├── groupversion_info.go        Group bruner.dev, version v1alpha1, SchemeBuilder, AddToScheme.
│       ├── common_types.go             Shared vocabulary: BrunerMode (Profile/Rollout/QualityGate/Shift),
│       │                               LocalExposure, ResourceProfile, condition type and reason constants.
│       ├── brunerapp_types.go          BrunerApp (namespaced). The only kind with an implemented reconciler.
│       ├── avtovaz_types.go            AvtoVAZ (cluster-scoped). Factory policy and default mode.
│       ├── togliatti_types.go          Togliatti (cluster-scoped). Observed node and runtime environment.
│       ├── konveer_types.go            Konveer (namespaced). Delivery workflow for an existing image.
│       ├── validation.go               Hand-written validation helpers shared by the CLI and controller.
│       ├── validation_test.go          Unit tests for those helpers.
│       └── zz_generated.deepcopy.go    controller-gen output. Never edited by hand.
│
├── cmd/
│   ├── manager/
│   │   └── main.go                     Controller-manager entrypoint: scheme registration, manager setup,
│   │                                   leader election flags, health and metrics endpoints,
│   │                                   BrunerAppReconciler registration.
│   └── brunerctl/
│       └── main.go                     CLI entrypoint. Delegates to internal/cli.Execute.
│
├── internal/
│   ├── controller/
│   │   ├── brunerapp_controller.go     The only reconciler in v0.1. Owns the Deployment and the
│   │                                   ClusterIP Service of a BrunerApp, evaluates AvtoVAZ policy,
│   │                                   writes conditions, stage and endpoint into status.
│   │                                   Togliatti, AvtoVAZ and Konveer reconcilers are deferred to v0.2
│   │                                   (see reconciliation.md).
│   │   └── brunerapp_controller_test.go
│   │                                   envtest suite for the reconciler.
│   │
│   ├── factory/                        Pure, deterministic logic. No API calls, no clock, no I/O.
│   │                                   This is where the unit tests concentrate.
│   │   ├── labels.go                   Label and selector construction, owner naming.
│   │   ├── build.go   + build_test.go  Desired Deployment and Service from a BrunerApp and a mode.
│   │   ├── policy.go  + policy_test.go AvtoVAZ policy evaluation into verdicts and warnings.
│   │   └── status.go  + status_test.go Stage derivation, condition assembly, status comparison.
│   │
│   └── cli/                            Everything brunerctl does.
│       ├── root.go                     Cobra command tree: install, uninstall, status, factory status,
│       │                               app deploy|list|inspect, konveer apply, events, diagnose.
│       ├── output.go                   Printer: OK / Warn / Fail / Info / DryRun, tables, summary blocks.
│       ├── kube.go                     Scheme, rest.Config and client construction, admin-kubeconfig
│       │                               fallback, node and deployment readiness helpers.
│       ├── versions.go                 Every pinned version, path, CIDR, port and minimum in one file.
│       ├── runner.go                   CommandRunner interface with ExecRunner and DryRunner.
│       ├── preflight.go                Host fact collection and the pure Preflight function.
│       ├── install.go                  The nine-step installation flow and the generated
│       │                               kubeadm, CNI and factory manifests.
│       ├── uninstall.go                The six-step removal flow.
│       ├── status.go                   status, factory status, events and diagnose implementations.
│       ├── app.go                      app deploy, list and inspect implementations.
│       └── konveer.go                  Client-side delivery workflow: validate -> apply -> rollout ->
│                                       verify, writing each stage into Konveer status.
│
├── config/
│   ├── crd/
│   │   └── bases/                      controller-gen output, applied by the Helm chart.
│   │       ├── bruner.dev_togliattis.yaml
│   │       ├── bruner.dev_avtovazs.yaml
│   │       ├── bruner.dev_konveers.yaml
│   │       └── bruner.dev_brunerapps.yaml
│   ├── rbac/
│   │   └── role.yaml                   ClusterRole brunernetes-manager. Reviewed in security.md.
│   └── samples/                        (empty) One valid sample per kind; api.md carries the
│                                       reference examples until these land.
│
├── charts/
│   └── brunernetes/
│       └── templates/                  (empty) Chart.yaml, values.yaml and the templates for CRDs,
│                                       ServiceAccount, ClusterRole, ClusterRoleBinding and the
│                                       controller Deployment. Installed by step 8 of the installer.
│
├── demo/
│   └── factory-showroom/               The demo workload. A static site, no backend, no state.
│       ├── Dockerfile                  nginx base plus the site. Built out of band; Brunernetes
│       │                               never builds images (ADR 0004).
│       ├── nginx.conf                  Serves on 8080, exposes /healthz.
│       ├── README.md
│       ├── site/                       index.html, styles.css, app.js, content.js, healthz.
│       └── qa/                         Rendered QA screenshots and QA_NOTES.md.
│
├── docs/                               This documentation set.
│   ├── adr/
│   │   ├── 0001-single-node-upstream-kubernetes.md
│   │   ├── 0002-crd-and-controller-boundary.md
│   │   ├── 0003-localhost-only-exposure.md
│   │   └── 0004-delivery-without-image-build.md
│   ├── architecture.md
│   ├── repository-structure.md
│   ├── api.md
│   ├── reconciliation.md
│   ├── installation.md
│   ├── roadmap.md
│   ├── testing.md
│   ├── security.md
│   └── open-questions.md
│
├── hack/
│   └── boilerplate.go.txt              Licence header injected into generated files.
│
├── test/
│   └── e2e/                            (empty) kind-based smoke test; see testing.md.
│
├── .github/
│   └── workflows/                      (empty) CI: build, vet, unit, envtest, helm lint, e2e.
│
├── go.mod
└── go.sum
```

## Not yet present

These are referenced by the documentation and the install flow and are expected
before v0.1 is complete. They are listed here so the gap is visible rather than
implied:

| Path | Purpose |
| --- | --- |
| `Makefile` | `generate`, `manifests`, `build`, `test`, `envtest`, `lint`, `helm-lint`, `e2e`, `demo` |
| `README.md` | Project overview and quickstart |
| `charts/brunernetes/Chart.yaml`, `values.yaml`, `templates/*` | The chart the installer expects at `charts/brunernetes` |
| `config/samples/bruner.dev_v1alpha1_*.yaml` | The sample referenced by `brunerctl install`'s closing hint |
| `test/e2e/*` | The kind-based smoke test |
| `.github/workflows/ci.yaml` | The CI matrix described in [testing.md](testing.md) |

## Layout principles

**`api/v1alpha1` is the only place a field is defined.** The CRD YAML, the
deepcopy functions and every documented default are derived from it. When this
documentation and the Go types disagree, the Go types win and the document is
wrong.

**`internal/factory` holds all deterministic logic.** Given a `BrunerApp` and a
`BrunerMode`, building the desired `Deployment` is a pure function; evaluating
`AvtoVAZ` policy is a pure function; deriving a stage from observed replica
counts is a pure function. Keeping these outside the reconciler is what makes
the interesting behaviour testable without an API server, and it is why the
controller file stays short enough to read in one sitting.

**`internal/controller` holds only the reconcile loop.** Fetch, compute with
`internal/factory`, write, decide the requeue. Nothing clever lives there.

**`internal/cli` is not a thin wrapper over the controller.** In v0.1 it carries
real responsibility: it runs the installer, and it executes the Konveer delivery
workflow client-side because no Konveer reconciler exists yet. That is a
deliberate, time-boxed arrangement documented in
[reconciliation.md](reconciliation.md) and scheduled for removal in
[roadmap.md](roadmap.md) v0.2.

**`internal/` rather than `pkg/`.** Nothing here is a public API. Marking it
`internal` makes that enforceable by the compiler instead of by convention.

## Related documents

- [architecture.md](architecture.md) - how these pieces interact at runtime
- [api.md](api.md) - the field-level reference for `api/v1alpha1`
- [reconciliation.md](reconciliation.md) - what each controller does and which ones exist
- [installation.md](installation.md) - what `internal/cli/install.go` performs
- [testing.md](testing.md) - where tests live and what they cover
