# Brunernetes

Brunernetes is a single-node Kubernetes distribution and application-delivery
MVP built on upstream Kubernetes. It bootstraps one Linux host into a working
cluster with `kubeadm` and `containerd`, installs four custom resources in the
`bruner.dev/v1alpha1` API group, runs one controller, and publishes exactly one
web application on the loopback interface. The `brunerctl` CLI installs the
environment, reports its state, deploys the application and diagnoses faults.
An AvtoVAZ/Togliatti factory metaphor is applied to naming only: the resource
kinds, profiles and status wording borrow from an assembly line, while every
message states the real technical condition first and the themed phrasing
second.

## What this is not

- **Not a Kubernetes fork.** No patched or vendored Kubernetes source.
- **Not a replacement for any `kube-*` component.** `kube-apiserver`,
  `kube-controller-manager`, `kube-scheduler`, `kubelet`, `kube-proxy`, `etcd`,
  CoreDNS and the CNI plugins are the pinned upstream binaries, unmodified.
  `brunerctl` is not a `kubectl` replacement either.
- **Not multi-node.** One host. There is no join flow, no HA control plane, no
  cluster federation.
- **Not an image builder and not a registry.** Images are built and pushed out
  of band; `Konveer` only validates and rolls out a reference that already
  exists.
- **Not a CI system.** There is no pipeline engine, no build triggers and no
  artifact store.
- **Not production-ready.** This is a v0.1 MVP: three of the four CRDs have no
  reconciler, there is no admission webhook, no TLS on the endpoint and no
  supported upgrade path.
- **No public ingress.** No ingress controller, no NodePort, no LoadBalancer.
  The application is bound to `127.0.0.1` and `allowPublicExposure` is a policy
  field whose only accepted value is `false`.

## Requirements

| Requirement | Value |
| --- | --- |
| Architecture | Linux x86_64 (amd64) only |
| Distribution | Ubuntu and Debian tested; other distributions produce a preflight warning |
| CPU | 2 cores or more |
| Memory | 4 GiB or more |
| Disk | 20 GiB or more free on the root file system |
| Privileges | root, because `install` and `uninstall` change the host |
| Swap | disabled (`swapoff -a`, and the `/etc/fstab` entry commented out) |
| cgroups | cgroup v2 mounted (`systemd.unified_cgroup_hierarchy=1`) |

`helm` must be on `PATH` for the controller chart to be installed; a missing
`helm` is a preflight warning, not a failure. The minimums above are the
constants in `internal/cli/versions.go`; CPU, memory, swap, cgroup v2, the
architecture, root privileges and the control-plane ports are blocking
preflight checks, while the disk figure, the distribution and the kernel
module and sysctl state are warnings the installer then fixes itself.

The installer pins one known-good component set: Kubernetes `v1.32.3`
(packages `1.32.3-1.1` from the `v1.32` channel of `pkgs.k8s.io`), CNI plugins
`v1.6.2`, `crictl` `v1.32.0` and the sandbox image
`registry.k8s.io/pause:3.10`. The pod network is `10.244.0.0/24` and the
service network is `10.96.0.0/16`.

## Quick start

Build the binaries:

```bash
make build
```

Plan the installation without touching the host. The dry run prints every
command, file and manifest the installer would apply:

```bash
sudo ./bin/brunerctl install --preset garage --dry-run
```

Run the real installation. It works through nine steps: preflight, kernel
modules, sysctls, pinned packages and containerd, the containerd systemd cgroup
driver, `kubeadm init`, removing the control-plane taint plus the bridge and
portmap CNI configuration, the Helm chart into `brunernetes-system`, and
finally the `Togliatti` and `AvtoVAZ` resources for the chosen preset. Reruns
converge instead of repeating `kubeadm init`:

```bash
sudo ./bin/brunerctl install --preset garage
```

Check what the factory reports about itself:

```bash
./bin/brunerctl status
./bin/brunerctl factory status
```

Build the demo image out of band and deploy it. Brunernetes never builds
images:

```bash
cd demo/factory-showroom && docker build -t factory-showroom:0.1.0 . && cd -

kubectl apply -f demo/manifests/namespace.yaml
./bin/brunerctl app deploy -f demo/manifests/brunerapp.yaml
./bin/brunerctl app inspect factory-showroom -n brunernetes-demo
```

Verify the loopback endpoint. The same URL from another host on the network is
refused:

```bash
curl -fsS http://127.0.0.1:30080/healthz
```

Optionally run the delivery workflow for the same image, which validates the
reference and the policy, applies the app, waits for the rollout and readiness,
and then re-checks the endpoint:

```bash
./bin/brunerctl konveer apply -f demo/manifests/konveer.yaml
```

Remove everything. This is destructive: the cluster, its workloads and its etcd
data are deleted, so the command requires `--yes`:

```bash
sudo ./bin/brunerctl uninstall --yes
```

## Custom resources

All four kinds live in the `bruner.dev/v1alpha1` API group and are defined in
`api/v1alpha1`. Only `BrunerApp` has a controller in v0.1; the other three are
accepted, stored and read, but nothing watches them.

| Kind | Scope | Purpose | Implemented reconciler |
| --- | --- | --- | --- |
| `Togliatti` | Cluster | The single-node factory environment: node, Kubernetes version, container runtime, pod network, capacity and loopback endpoint capability | No |
| `AvtoVAZ` | Cluster | Factory-wide configuration: the default `BrunerMode` and the safety and quality policy every application is measured against | No |
| `BrunerApp` | Namespaced | The one web application. The controller owns a Deployment and a ClusterIP Service, applies defaults, evaluates policy and writes conditions, stage and endpoints | Yes |
| `Konveer` | Namespaced | Delivery workflow for an existing image: validate, apply, roll out, verify. Never builds images | No, the stages run client-side in `brunerctl konveer apply` |

Short names are available: `tgl`/`factory`, `vaz`, `bapp`/`app` and
`kvr`/`line`, all in the `brunernetes` category. Field-level documentation is
in [docs/api.md](docs/api.md); one runnable sample per kind is in
[config/samples](config/samples).

## brunerctl command reference

`brunerctl` accepts two persistent flags: `--kubeconfig` (defaults to
`KUBECONFIG`, then the user kubeconfig, then the kubeadm admin kubeconfig) and
`--namespace`/`-n` (defaults to `default`).

| Command | What it does |
| --- | --- |
| `brunerctl install` | Installs single-node Kubernetes with kubeadm and pinned upstream components, then the Brunernetes CRDs and controller with Helm. Requires root. Flags: `--preset`, `--dry-run`, `--skip-preflight`, `--skip-packages`, `--skip-cni`, `--skip-controller`, `--chart`, `--kubernetes-version`, `--pod-cidr`, `--api-advertise-address` |
| `brunerctl uninstall` | Removes the controller release, resets the control plane with `kubeadm reset` and deletes the generated files. Requires root and `--yes`. Flags: `--dry-run`, `--keep-packages`, `--purge`, `--yes` |
| `brunerctl status` | Shows the state of the factory and its application in one summary |
| `brunerctl factory status` | Shows the `Togliatti` factory environment in detail |
| `brunerctl app deploy` | Applies a `BrunerApp` manifest. Flags: `--filename`/`-f` (`-` reads stdin), `--wait` (default 3m, `0` returns immediately) |
| `brunerctl app list` | Lists the applications of the factory. Alias `ls`; the `app` command itself is also reachable as `apps` |
| `brunerctl app inspect <name>` | Shows everything known about one application |
| `brunerctl konveer apply` | Applies a `Konveer` manifest and runs the delivery. Flag: `--filename`/`-f` |
| `brunerctl events` | Shows recent events of the Brunernetes resources. Flags: `--limit` (default 30), `--all-namespaces`/`-A` |
| `brunerctl diagnose` | Checks the whole installation and reports the first broken thing |

## Operating modes

`BrunerMode` has four fields. `AvtoVAZ.spec.defaultMode` sets the factory
default and `BrunerApp.spec.mode` overrides it field by field; anything still
unset falls back to `profile: garage`, `rollout: steady`,
`qualityGate: practical`, `shift: day`. `brunerctl install --preset` accepts the
four profile names and writes the matching mode into the generated `AvtoVAZ`,
where the `strict` preset also sets `qualityGate: strict`,
`requireReadinessProbe: true` and `requireResourceRequests: true`.

| Field | Value | What it actually changes |
| --- | --- | --- |
| `profile` | `garage` | The default. No extra requirement, no extra delay: a missing readiness probe or missing resource requests is a `PolicyWarning` condition, not a block |
| `profile` | `showroom` | Sets `minReadySeconds: 5` on the Deployment, so a new replica must hold ready for five seconds before it counts as available and a demonstration does not flip between states |
| `profile` | `strict` | Turns the readiness-probe and resource-request requirements into blocking policy violations that hold the rollout back |
| `profile` | `friday` | Holds an image change on an existing application until `spec.override.approved` is `true` with an `override.image` matching `spec.image`. The first rollout is not guarded |
| `rollout` | `steady` | The only supported value. Produces a `RollingUpdate` strategy with `maxSurge: 0` and `maxUnavailable: 1`, so on a small host the old replica leaves before the new one arrives, plus `revisionHistoryLimit: 3` and `progressDeadlineSeconds: 300` |
| `qualityGate` | `practical` | Policy problems are reported as warnings and the rollout continues |
| `qualityGate` | `strict` | The readiness-probe and resource-request requirements become blocking violations, exactly as `profile: strict` |
| `shift` | `day` | Periodic resync of a healthy `BrunerApp` every 30 seconds |
| `shift` | `night` | Periodic resync every 2 minutes, which reduces reporting noise. Nothing else changes |

Independently of the mode, `AvtoVAZ.spec.policy` controls
`requireReadinessProbe`, `requireResourceRequests`, `latestTagPolicy`
(`Ignore`, `Warn` or `Reject` for a mutable tag such as `:latest`),
`maxApplications` (the MVP accepts only `1`), `allowedRegistries` (an empty
list allows any registry; a non-empty list is always blocking) and
`allowPublicExposure`, which is permanently `false`.

## How local exposure works

The `BrunerApp` controller always creates a **ClusterIP** Service on port `80`,
targeting the named container port `http`. It never creates a `NodePort` or a
`LoadBalancer` Service, and there is no ingress controller anywhere in the
installation.

Host access is a separate mechanism: when `spec.localExposure.enabled` is true,
the pod's container port carries `hostIP: 127.0.0.1` and
`hostPort: 30080` (both configurable, port range 1024-65535). The installer
writes a minimal CNI conflist to `/etc/cni/net.d/10-brunernetes.conflist` with
`bridge`, `portmap` and `loopback`, in that order, with the `portMappings`
capability enabled; the **portmap** plugin is what actually programs the
host-port mapping. The result is `http://127.0.0.1:30080`, reported in
`status.localEndpoint`, next to the in-cluster
`http://<service>.<namespace>.svc.cluster.local:80` in
`status.clusterIPEndpoint`.

A `hostIP` outside `127.0.0.0/8` is not honoured. The CRD pattern rejects
anything that is not `127.x.x.x`, and the controller additionally normalises a
non-loopback value back to `127.0.0.1` and records a policy warning. The
rationale is in [ADR 0003](docs/adr/0003-localhost-only-exposure.md).

## Repository layout

```
.
├── api/v1alpha1/        API types for all four kinds; the source of truth for every field
├── cmd/
│   ├── manager/         Controller-manager entrypoint
│   └── brunerctl/       CLI entrypoint
├── internal/
│   ├── controller/      The BrunerApp reconciler, the only one in v0.1
│   ├── factory/         Pure logic: labels, desired objects, policy evaluation, status
│   └── cli/             Everything brunerctl does: install, uninstall, status, app, konveer
├── config/
│   ├── crd/bases/       controller-gen output, applied by the Helm chart
│   ├── rbac/            ClusterRole for the manager
│   └── samples/         One sample per kind plus a kustomization
├── charts/brunernetes/  Helm chart for CRDs, RBAC and the controller
├── demo/
│   ├── factory-showroom/ The static demo workload and its Dockerfile
│   └── manifests/        Namespace, BrunerApp and Konveer for the demo
├── docs/                Architecture, API, installation, reconciliation, testing, security, ADRs
├── hack/                Generated-file boilerplate header
└── test/e2e/            kind-based smoke test
```

## Development

The Go module is `github.com/brunka09/brunernetes` on Go 1.24, with
`k8s.io/*` v0.32.3, `sigs.k8s.io/controller-runtime` v0.20.4 and
`github.com/spf13/cobra` v1.8.1.

| Make target | What it does |
| --- | --- |
| `make build` | Builds `bin/brunerctl` and `bin/manager` |
| `make test` | Runs the unit tests |
| `make lint` | Runs the formatting and static-analysis checks |
| `make generate` | Regenerates the deepcopy functions with controller-gen |
| `make manifests` | Regenerates the CRDs in `config/crd/bases` and the RBAC role |
| `make install` | Installs the environment on this host with `brunerctl install` |
| `make uninstall` | Removes it again with `brunerctl uninstall` |
| `make demo` | Applies the demo namespace and deploys the demo application. The image is built separately with `make demo-image` |
| `make test-envtest` | Runs the controller integration suite against a real API server, installing the binaries first |
| `make envtest` | Downloads the `kube-apiserver` and `etcd` binaries the integration suite needs |
| `make helm-lint` | Lints the Helm chart |
| `make kind-e2e` | Runs the kind-based end-to-end smoke test |

Run the fast tests directly:

```bash
go test ./...
```

The unit tests in `internal/factory`, `api/v1alpha1` and `internal/cli` need no
dependencies: building a Deployment, evaluating policy, deriving a stage and
running preflight are pure functions over constructed inputs. The envtest suite
in `internal/controller` needs real `kube-apiserver` and `etcd` binaries and
**skips itself when `KUBEBUILDER_ASSETS` is not set**, so `go test ./...` stays
green on a machine without them. Point it at the binaries to run it:

```bash
make test-envtest
```

Regenerate code and manifests after changing anything in `api/v1alpha1`;
`zz_generated.deepcopy.go` and `config/crd/bases` are controller-gen output and
are never edited by hand. The full strategy, including what envtest can and
cannot assert, is in [docs/testing.md](docs/testing.md).

## Status and roadmap

v0.1 is the phase currently in progress, and what exists today is what this
README describes: the four CRDs, the `BrunerApp` reconciler, the `brunerctl`
install and uninstall flows with preflight and dry run, the read and diagnose
commands, the client-side Konveer delivery, loopback-only exposure through
ClusterIP plus portmap, and the Factory Showroom demo workload. Explicitly not
in v0.1: reconcilers for `Togliatti`, `AvtoVAZ` and `Konveer`, admission
webhooks, image building, more than one node, more than one application, and
TLS on the endpoint.

v0.2 moves the remaining reconcilers into the controller, including the Konveer
delivery workflow, and adds a small optional extra: **"Build the Konveer"**, a
browser mini-game served by the demo workload in which the player assembles a
delivery conveyor from the validate, apply, rollout and verify stages under the
same constraints the real controller applies. It is a teaching device with a
score, not a platform feature, and it is not part of this MVP. v0.3 is
hardening: privilege reduction, supply chain, optional loopback TLS and a
documented upgrade path.

The full phase breakdown, with checkable exit criteria and per-phase non-goals,
is in [docs/roadmap.md](docs/roadmap.md).

## Documentation

- [docs/architecture.md](docs/architecture.md) - how the components interact at runtime
- [docs/repository-structure.md](docs/repository-structure.md) - annotated repository layout
- [docs/api.md](docs/api.md) - field-level reference for `bruner.dev/v1alpha1`
- [docs/reconciliation.md](docs/reconciliation.md) - what each controller does and which ones exist
- [docs/installation.md](docs/installation.md) - the install and uninstall flows in detail
- [docs/testing.md](docs/testing.md) - the four test layers and what each covers
- [docs/security.md](docs/security.md) - threat model, RBAC review and the risk register
- [docs/roadmap.md](docs/roadmap.md) - v0.1, v0.2 and v0.3 scope and exit criteria
- [docs/open-questions.md](docs/open-questions.md) - decisions that are still open
- [docs/adr/0001-single-node-upstream-kubernetes.md](docs/adr/0001-single-node-upstream-kubernetes.md) - single node on upstream Kubernetes
- [docs/adr/0002-crd-and-controller-boundary.md](docs/adr/0002-crd-and-controller-boundary.md) - the CRD and controller boundary
- [docs/adr/0003-localhost-only-exposure.md](docs/adr/0003-localhost-only-exposure.md) - localhost-only exposure
- [docs/adr/0004-delivery-without-image-build.md](docs/adr/0004-delivery-without-image-build.md) - delivery without an image build
- [demo/factory-showroom/README.md](demo/factory-showroom/README.md) - the demo workload

## License

Brunernetes is released under the MIT License: permissive use, modification and
redistribution, with the copyright notice retained and no warranty of any kind.
The full text is in [LICENSE](LICENSE).

The pinned upstream components Brunernetes installs keep their own licenses:
Kubernetes and the containernetworking plugins are Apache License 2.0, and
containerd is Apache License 2.0. Brunernetes does not redistribute them; the
installer fetches them from `pkgs.k8s.io` and the upstream release artifacts.
