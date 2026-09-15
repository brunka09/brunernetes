# Roadmap

Three phases. Each has a scope, exit criteria that can be checked rather than
argued about, and explicit non-goals. A non-goal is not a promise for later; it
is a statement that the phase is complete without it.

The guiding constraint across all three phases: Brunernetes runs upstream
Kubernetes on one machine and exposes exactly one application on
`127.0.0.1:30080`. Nothing on this roadmap changes that.

| Phase | Theme | State |
| --- | --- | --- |
| v0.1 | One application, end to end, on one host | In progress |
| v0.2 | The remaining reconcilers, and the delivery workflow moved server-side | Planned |
| v0.3 | Hardening, supply chain and operability | Planned |

## v0.1 - The line runs

**Goal:** a single command on a clean Ubuntu or Debian host produces a working
single-node Kubernetes cluster, a running Brunernetes controller, and a demo
application reachable at `http://127.0.0.1:30080`, and a second command removes
all of it.

### Scope

- Four CRDs in `bruner.dev/v1alpha1`: `Togliatti` and `AvtoVAZ` cluster-scoped,
  `BrunerApp` and `Konveer` namespaced ([api.md](api.md)).
- One implemented reconciler: `BrunerApp`. It owns a `Deployment` and a
  ClusterIP `Service`, evaluates `AvtoVAZ` policy, and writes conditions, stage
  and the local endpoint into status ([reconciliation.md](reconciliation.md)).
- `brunerctl install` / `uninstall` with preflight, pinned versions, dry-run and
  idempotent reruns ([installation.md](installation.md)).
- `brunerctl status`, `factory status`, `app deploy|list|inspect`, `events`,
  `diagnose`.
- `brunerctl konveer apply` running the delivery stages client-side -
  validate, apply, rollout, verify - and writing the result into `Konveer`
  status. This is explicitly a v0.1 arrangement; see v0.2.
- Local exposure as ClusterIP plus a `hostIP: 127.0.0.1` / `hostPort: 30080`
  binding through the CNI portmap plugin
  ([ADR 0003](adr/0003-localhost-only-exposure.md)).
- Helm chart installing the CRDs, RBAC and controller into
  `brunernetes-system`.
- The Factory Showroom demo workload under `demo/factory-showroom`.
- This documentation set.

### Exit criteria

1. `sudo brunerctl install --preset garage --dry-run` on a clean host prints a
   complete plan and changes nothing.
2. `sudo brunerctl install --preset garage` on a clean Ubuntu 22.04 or 24.04 host
   finishes without a `FAIL`, and `kubectl get nodes` shows one `Ready` node.
3. Rerunning the same command is a no-op convergence: no `kubeadm init`, no
   errors, no duplicate resources.
4. Applying the sample `BrunerApp` produces a `Deployment` and a ClusterIP
   `Service`, and the resource reaches `stage: Showroom` with `Available=True`.
5. `curl -fsS http://127.0.0.1:30080/healthz` returns HTTP 200.
6. The same URL from another host on the network is refused or times out.
7. A policy violation under the `strict` preset surfaces as
   `PolicyWarning=True` with a reason and an event, and never as a silent
   rejection.
8. `brunerctl konveer apply -f konveer.yaml` records four stages in status with
   correct outcomes, including a failed rollout.
9. `brunerctl uninstall --purge` leaves no Brunernetes files under `/etc`, no
   Kubernetes packages, and `kubectl get nodes` fails.
10. Unit tests, `go vet`, `helm lint` and the kind smoke test pass in CI
    ([testing.md](testing.md)).

### Non-goals for v0.1

- Reconcilers for `Togliatti`, `AvtoVAZ` and `Konveer`. The CRDs exist and are
  written to; nothing watches them.
- Any admission webhook ([ADR 0002](adr/0002-crd-and-controller-boundary.md)).
- Image building or a registry ([ADR 0004](adr/0004-delivery-without-image-build.md)).
- More than one node, HA, or any form of cluster join.
- TLS on the loopback endpoint.
- Persistent storage, StatefulSets, Jobs, CronJobs.
- Multiple applications per factory (`maxApplications: 1`).
- Anything other than `amd64` on Ubuntu or Debian.
- Prometheus metrics beyond the controller-runtime defaults.

## v0.2 - The factory reports on itself

**Goal:** the state a user sees comes from controllers watching the cluster, not
from a CLI that computed it at the moment it was asked. Plus the first piece of
the project that exists to be enjoyable rather than useful.

### Scope

**Togliatti reconciler.** Watches the single `Node`, the `kube-system` control
plane pods and the node's `NodeInfo`. Writes `status.kubernetesVersion`,
`containerRuntime`, `nodeName`, `capacity`, `components` and the
`EnvironmentReady` condition. Requeues on `spec.probeIntervalSeconds`. This
turns `brunerctl factory status` from a live inspection into a status read.

**AvtoVAZ reconciler.** Watches `AvtoVAZ`, `Togliatti` and all `BrunerApp`
resources. Writes `status.managedApplications`, `status.factoryReady` and the
`PolicyAccepted` condition, including the case where `spec.factoryRef` points at
a `Togliatti` that does not exist - currently an error that only surfaces per
application.

**Konveer reconciler.** Moves the delivery workflow from
`internal/cli/konveer.go` into a controller: the same four stages, the same
status structure, but driven by the reconcile loop, so a delivery survives the
operator closing their terminal and a stalled rollout is observed rather than
waited on. `brunerctl konveer apply` becomes what it should have been from the
start: apply the manifest, then watch status. This is the single largest source
of behavioural risk in v0.2, because the client-side and server-side
implementations must produce identical status for the same input during the
transition.

**Multiple applications.** Raise the `maxApplications` ceiling and make the
`hostPort` allocation explicit instead of a single well-known number. This
requires a decision recorded in [open-questions.md](open-questions.md).

**"Build the Konveer" browser mini-game.** A small single-page game served by
the demo workload, alongside the Factory Showroom, at a path such as
`/konveer`. The player assembles a delivery conveyor from stages - validate,
apply, rollout, verify - under constraints that mirror the real ones: a
mutable `:latest` tag costs you a quality point, a missing readiness probe makes
the rollout stall, `maxSurge: 0` means the old part leaves the belt before the
new one arrives. It is a teaching device with a score, not a feature of the
platform: no backend, no state outside the browser tab, no coupling to the
controller, and it must be removable by deleting one directory. Its purpose is
that someone who plays it for five minutes understands why a `steady` rollout
behaves the way it does. Explicitly not in scope for the game: multiplayer,
persistence, leaderboards, or any network call.

**Observability.** Controller-specific Prometheus metrics: reconcile duration
and outcome per kind, policy verdict counts, delivery stage outcomes. A Grafana
dashboard JSON in the repository.

**Quality of life.** `brunerctl logs`, `brunerctl app delete`, shell completion,
`--output json` on the read commands.

### Exit criteria

1. `brunerctl factory status` reads exclusively from `Togliatti.status`; removing
   the live node inspection from the CLI changes no output.
2. `AvtoVAZ.status.managedApplications` tracks `BrunerApp` creation and deletion
   within one reconcile period.
3. A `Konveer` applied with `kubectl apply` alone - no `brunerctl` - completes
   all four stages and reaches `Delivered=True`.
4. The client-side and controller-side Konveer implementations produce identical
   status for a shared fixture set, verified by a test, before the client-side
   path is deleted.
5. Two `BrunerApp` resources in one namespace are exposed on distinct host ports
   without manual coordination.
6. The mini-game loads offline, makes no network request after the initial page
   load, and is removed cleanly by deleting its directory.
7. envtest coverage exists for every new controller.

### Non-goals for v0.2

- Multi-node. Still one host.
- Admission webhooks. Policy stays advisory, in the controller.
- Image building. Still none.
- Authentication or TLS on the loopback endpoint - that is v0.3.
- API version `v1beta1`. The API stays `v1alpha1` while reconcilers are still
  being added.

## v0.3 - Hardening

**Goal:** the properties that make a platform safe to hand to someone else:
bounded privilege, a verifiable supply chain, predictable upgrades, and an
honest security posture.

### Scope

**Privilege reduction.** Split the single `brunernetes-manager` `ClusterRole`
into a narrow cluster-scoped role for the cluster-scoped kinds and a namespaced
`Role` for workload management, bound only where the controller actually manages
applications. Drop `pods` read access if the observed state can be derived from
the `Deployment` alone. Run the manager as a non-root user with a read-only root
filesystem, and add `runAsNonRoot` to the workload pod template - currently
absent from the built security context.

**Supply chain.** Pin the controller image by digest rather than tag. Sign
releases and generate an SBOM. Verify the CNI plugin tarball checksum during
installation instead of trusting the download. Make `latestTagPolicy: Reject`
the default for the `strict` preset.

**Transport security.** An optional TLS terminator on the loopback endpoint with
a self-signed certificate generated at install time, so the demo stops teaching
that plaintext is fine. Off by default; enabling it must be one flag.

**Upgrades.** A documented and tested path from one pinned Kubernetes patch
version to the next, including what happens to a running application during the
kubelet restart. An `AvtoVAZ`-driven drain and restore is the intended shape.

**API stability.** Promote `bruner.dev/v1alpha1` to `v1beta1` with a conversion
path, once the field set has survived v0.2 without change.

**Resilience.** Controller restart under load, `kubeadm init` failure recovery,
etcd disk exhaustion, an image that never pulls, a node that goes `NotReady`
mid-rollout - each with a defined observable outcome and a test.

**Documentation.** A troubleshooting guide derived from the failure modes
`brunerctl diagnose` already knows how to detect.

### Exit criteria

1. The controller runs as non-root with a read-only root filesystem and no
   capability beyond the default drop-all.
2. No `ClusterRole` grants write access to a resource the controller does not
   create.
3. The release pipeline produces a signed image referenced by digest and an
   SBOM, and the installer verifies the CNI tarball checksum.
4. A documented upgrade from `v1.32.x` to `v1.32.y` completes with the
   application's downtime measured and stated.
5. Every risk in [security.md](security.md) is either mitigated or has a
   residual risk that was consciously accepted and recorded.
6. A chaos-style test suite covers the failure modes above.

### Non-goals for v0.3

- Multi-node, HA control plane, or cluster federation. If that is wanted, the
  answer is a different product ([ADR 0001](adr/0001-single-node-upstream-kubernetes.md)).
- A general-purpose PaaS. `BrunerApp` stays a stateless HTTP workload.
- Public exposure. `allowPublicExposure` remains a policy field that defaults
  to `false`, and nothing in v0.3 makes `true` a supported configuration.
- Cloud provider integration, storage classes, or external load balancers.
- A web UI for the platform itself. The mini-game is not a precedent.

## Related documents

- [architecture.md](architecture.md)
- [reconciliation.md](reconciliation.md) - current implementation status per controller
- [open-questions.md](open-questions.md) - decisions that gate v0.2
- [security.md](security.md) - the risk register v0.3 works through
- [testing.md](testing.md)
