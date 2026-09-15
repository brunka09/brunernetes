# Test strategy

Brunernetes has one property that shapes its whole test strategy: almost all of
the interesting behaviour is a pure function of inputs the test can construct.
Building a `Deployment` from a `BrunerApp`, evaluating `AvtoVAZ` policy,
deriving a stage from replica counts, and deciding whether a host passes
preflight are all deterministic transformations with no clock, no network and no
API server. The strategy is to keep them that way and test them cheaply, then
spend the expensive test budget only on the things that genuinely need a running
API server or a real cluster.

Four layers, in increasing cost and decreasing count:

| Layer | Tool | Runs in | Scope |
| --- | --- | --- | --- |
| Unit | `go test` | Seconds, no dependencies | `internal/factory`, `api/v1alpha1/validation.go`, `internal/cli` pure logic |
| Controller integration | envtest | Tens of seconds, needs control-plane binaries | `internal/controller` against a real API server |
| Packaging | `helm lint`, `helm template` | Seconds | `charts/brunernetes` |
| End to end | kind | Minutes | The controller, CRDs and chart on a real cluster |

## Unit tests

**`internal/factory` - the core of the suite.**

`build.go`: given a `BrunerApp` and a `BrunerMode`, assert the exact desired
`Deployment` and `Service`. The cases that matter are the ones where a default
is applied or a mode changes the output:

- Port defaults to `8080`, health path to `/healthz`, host IP to `127.0.0.1`,
  host port to `30080`, Service port to `80`, port name `http`.
- `revisionHistoryLimit: 3`, `progressDeadlineSeconds: 300`.
- Resource requests `50m` / `64Mi` and limits `500m` / `256Mi` when no profile
  is given, and the profile's values when one is.
- Readiness probe `2/5/2/1/3` and liveness probe `10/20/3/1/5` against the
  resolved health path.
- `rollout: steady` produces `RollingUpdate` with `maxSurge: 0` and
  `maxUnavailable: 1`.
- `profile: showroom` sets `minReadySeconds: 5`; every other profile leaves
  it at `0`.
- The Service is always `ClusterIP`, even if the input asks otherwise.
- The security context drops `ALL` capabilities, disables privilege escalation
  and sets the `RuntimeDefault` seccomp profile.
- The container is named `application` regardless of the application name.
- The selector is stable: building twice from the same input yields byte-identical
  labels.

`policy.go`: a table over `AvtoVAZ` policy fields crossed with `BrunerApp`
shapes. `requireReadinessProbe` and `requireResourceRequests` under the `strict`
preset versus the others; `latestTagPolicy` at `Allow`, `Warn` and `Reject`
against `:latest`, an explicit tag and a digest reference; `maxApplications`
at and above the limit; `allowPublicExposure: false` against a request for a
non-loopback host IP. The assertion is on the verdict *and* the reason string,
because the reason becomes a condition reason and an event the user reads.

`status.go`: stage derivation from observed replica counts -
`Blueprint` before anything exists, `Assembly` while replicas are converging,
`Showroom` when ready equals desired, `Breakdown` on failure. And `equalStatus`,
which is what prevents the controller from writing an identical status on every
reconcile: it must return `true` for semantically identical statuses that differ
in condition `lastTransitionTime`, and `false` for any real change.

`labels.go`: label and selector construction, name derivation, and the
truncation behaviour for names near the 63-character label limit.

**`api/v1alpha1/validation.go`** already has `validation_test.go`. Image
reference parsing is the part worth being thorough about: a bare name, a name
with a tag, a name with a digest, a registry with a port that is not a tag
separator, and an empty string.

**`internal/cli/preflight.go`** - `Preflight` is a pure function over
`HostFacts`, which is exactly why `CollectHostFacts` is separate from it.
Construct synthetic hosts and assert the level and remedy of each result: wrong
architecture, non-Debian distribution, non-root, one CPU, 3 GiB of memory, swap
on, no cgroup v2, a busy port with and without an existing cluster, missing
helm. The parser helpers - `readOSRelease`, `readMemoryGiB`, `readSwapEnabled` -
take a path argument, so they are tested against fixture files rather than the
real `/proc`.

**`internal/cli/install.go` generators.** `FactoryManifest`, the kubeadm config
and the CNI conflist are string generators with no side effects. Assert the CNI
conflist parses as JSON and has `bridge`, `portmap`, `loopback` in that order
with `portMappings` capability set; assert the kubeadm config contains
`cgroupDriver: systemd`, `failSwapOn: true` and the pinned version; assert each
preset produces the documented `profile` / `rollout` / `qualityGate` / `shift`
and the correct `requireReadinessProbe` and `requireResourceRequests`.

**`internal/cli/runner.go`.** `DryRunner` is itself a test double. A fake
`CommandRunner` that records calls lets the whole install flow be asserted as an
ordered command sequence without touching the host - which is the only practical
way to test step ordering, skip flags and the existing-cluster branch.

## Controller integration tests (envtest)

envtest starts a real `kube-apiserver` and `etcd` with no kubelet and no
scheduler. Pods are never scheduled and never become ready, so the test asserts
what the controller *writes*, not what the cluster converges to. That boundary
is what makes envtest the right tool for reconciler logic and the wrong tool for
rollout behaviour.

For the `BrunerApp` reconciler - the only one implemented in v0.1
([reconciliation.md](reconciliation.md)):

- Creating a `BrunerApp` creates a `Deployment` and a `Service` with the correct
  owner reference, and the `Service` is `ClusterIP`.
- Deleting the `BrunerApp` removes both by garbage collection.
- Reconciling twice without an input change performs no second status write -
  asserted on `metadata.resourceVersion`, which is the observable form of the
  `equalStatus` guard.
- A manual edit to the owned `Deployment` is reverted on the next reconcile,
  except for the selector, which is immutable and written only at creation.
- Changing `spec.image` updates the container image in place; it does not
  recreate the `Deployment`.
- A policy violation sets `PolicyWarning=True` with the expected reason and
  emits exactly one event; a second reconcile with the same verdict emits none;
  fixing the violation emits one more.
- The condition set is complete and consistent: `Reconciling`, `Available`,
  `Degraded`, `PolicyWarning`, each with `observedGeneration` matching.
- A missing `AvtoVAZ` leaves the application reconcilable with a stated
  degraded reason rather than a crash.
- `status.endpoint` is `http://127.0.0.1:30080` for the default exposure.

Requeue intervals - 30 s day shift, 2 min night shift, 15 s on fault - are
asserted on the returned `ctrl.Result` from a directly invoked `Reconcile`, not
by waiting.

## End-to-end smoke test (kind)

One test, in `test/e2e`, on a kind cluster. It is a smoke test by intent: it
proves the pieces fit together, and it is deliberately not where behaviour is
covered in depth.

1. Create a kind cluster with `extraPortMappings` for the loopback endpoint.
2. `helm install` the chart from the repository; wait for the controller to be
   ready.
3. Apply a `BrunerApp` using a small public image.
4. Wait for `stage: Showroom` and `Available=True`.
5. `curl` the endpoint and assert HTTP 200.
6. Apply a `Konveer` and assert all four stages reach `Succeeded`.
7. Apply a policy-violating `BrunerApp` and assert `PolicyWarning=True` - and
   assert the `Deployment` is still created, because policy is advisory
   ([ADR 0002](adr/0002-crd-and-controller-boundary.md)).
8. Delete both and assert the owned resources disappear.
9. Delete the cluster.

The test must be re-runnable on a dirty machine and must clean up on failure.

**kind is not the install target.** The installer bootstraps a host with
kubeadm; kind runs nodes as containers. The e2e test therefore exercises the
controller, the CRDs and the chart, and says nothing about `brunerctl install`.
That gap is covered by the disposable-VM check below, not by CI.

## Helm chart checks

- `helm lint charts/brunernetes`.
- `helm template` with default values, then with `--set` for the image
  repository, tag and resource overrides, piped through `kubectl apply
  --dry-run=client` to catch schema errors.
- Assert the rendered `ClusterRole` matches `config/rbac/role.yaml`, so
  generated RBAC and shipped RBAC cannot drift apart silently.
- Assert the rendered CRDs match `config/crd/bases/`.

## Generated code freshness

`make generate manifests` followed by `git diff --exit-code`. If `controller-gen`
output differs from what is committed, the build fails. This is the cheapest
possible defence against a field existing in Go but not in the CRD the API
server enforces.

## CI matrix

| Job | Runs on | Command | Gate |
| --- | --- | --- | --- |
| `build` | ubuntu-latest, Go 1.24.6 | `go build ./...` | Every push and PR |
| `vet` | ubuntu-latest | `go vet ./...` | Every push and PR |
| `lint` | ubuntu-latest | `golangci-lint run` | Every push and PR |
| `generate-diff` | ubuntu-latest | `make generate manifests && git diff --exit-code` | Every push and PR |
| `unit` | ubuntu-latest | `go test ./... -race -coverprofile=cover.out` | Every push and PR |
| `envtest` | ubuntu-latest | `make envtest` with Kubernetes 1.32 binaries | Every push and PR |
| `helm` | ubuntu-latest | `helm lint` and `helm template` | On chart or CRD change |
| `e2e` | ubuntu-latest | kind 1.32 smoke test | PRs to `main` and nightly |

Single Go version, single Kubernetes version, single architecture. A matrix
across versions would be dishonest here: the product pins Kubernetes `v1.32.3`
and supports `amd64` on Ubuntu and Debian only, so testing anything else would
assert support that is not offered. When the pin moves, the CI version moves
with it in the same commit.

`-race` on the unit job matters more than it looks: the controller shares a
cache-backed client across goroutines, and the CLI runs probes with timeouts.

## What is deliberately not tested, and why

| Not tested | Reason |
| --- | --- |
| `brunerctl install` against a real host in CI | It requires root, modifies `/etc`, installs packages and runs `kubeadm init`. A CI runner that survives it is not a representative host. Covered by a manual disposable-VM check before each release, plus unit tests of the ordered command sequence via a fake runner. |
| `kubeadm`, `containerd`, `kube-proxy`, CoreDNS, the CNI plugins | Upstream components with their own test suites. Testing them here would test someone else's code and pin us to their internals. We test that we configure them correctly, not that they work. |
| The real `/proc` and `/sys` readers in `CollectHostFacts` | They read the actual host, so their output is not reproducible in CI. The parsers take a path argument and are tested against fixtures; the collection function is a thin composition over them. |
| Multi-node behaviour | Out of scope by design ([ADR 0001](adr/0001-single-node-upstream-kubernetes.md)). There is no supported configuration to test. |
| Non-`amd64` architectures, non-Debian distributions | Unsupported and blocked or warned by preflight. |
| Actual off-host unreachability of the loopback endpoint | Requires a second machine on a real network. kind's networking does not model it faithfully, and a test that passes for the wrong reason is worse than no test. It is a documented manual verification step in [installation.md](installation.md). |
| Reconcilers for `Togliatti`, `AvtoVAZ` and `Konveer` | They do not exist in v0.1. Their envtest suites land with the controllers in v0.2 ([roadmap.md](roadmap.md)). |
| The client-side Konveer workflow's network probe | It probes `127.0.0.1:30080`, which is not available in envtest. Stage transitions are unit tested with an injected prober; the real probe is covered by the kind e2e test. |
| Performance and scale | With `maxApplications: 1` on one node there is no scale dimension worth measuring, and any number published from a single developer machine would be a benchmark in name only. |

## Coverage expectations

No global percentage target - it rewards testing the wrong things. The
expectation is stated per package:

- `internal/factory` and `api/v1alpha1/validation.go`: every branch. These are
  pure, cheap to test, and every branch is a user-visible behaviour.
- `internal/cli` pure logic - preflight, generators, the ordered command
  sequence: every branch.
- `internal/controller`: every condition transition and every requeue decision,
  via envtest.
- `internal/cli` I/O paths, `cmd/`: not measured. Covered incidentally by e2e.

## Related documents

- [repository-structure.md](repository-structure.md) - where the code under test lives
- [reconciliation.md](reconciliation.md) - the behaviour the envtest suite asserts
- [api.md](api.md) - the defaults and validation the unit tests pin
- [installation.md](installation.md) - the manual verification steps
- [roadmap.md](roadmap.md)
