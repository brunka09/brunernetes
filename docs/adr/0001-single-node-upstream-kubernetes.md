# ADR 0001: Single node on upstream Kubernetes bootstrapped with kubeadm

- Status: Accepted
- Date: 2026-09-07
- Deciders: Brunernetes maintainers
- Applies to: v0.1 (MVP)

## Context

Brunernetes is a single-node Kubernetes distribution and developer experience
layer. It installs Kubernetes on one Linux host, runs exactly one demo web
application, publishes it on the loopback interface, and adds four custom
resources, a controller manager and the `brunerctl` CLI on top.

The project has two properties that dominate every technical choice:

1. The value of the project is the API surface, the controllers and the
   operator experience - not the cluster plumbing underneath it.
2. The project must be honest about what it is. It is explicitly **not** a
   Kubernetes clone, a fork, or a re-implementation of any control-plane
   component. The specification forbids modifying kube-apiserver, kubelet,
   kube-scheduler and kube-proxy.

That means the substrate must be a real, boring, upstream Kubernetes cluster
that a platform engineer already recognises, with kubeadm-standard file
locations, standard static pod manifests, standard component flags and standard
troubleshooting procedures. Anything that behaves "almost like Kubernetes"
would poison every diagnostic the controllers and the CLI produce, because
themed diagnostics are only useful if the underlying technical condition maps
1:1 to what upstream documentation describes.

The target host profile is modest: Ubuntu or Debian, x86_64, 2 vCPU, 4 GB RAM,
20 GB disk, containerd as the runtime.

## Decision

Brunernetes bootstraps a **single-node cluster with `kubeadm init`, using
unmodified upstream Kubernetes components at pinned versions**:

- `kube-apiserver`, `kube-controller-manager`, `kube-scheduler` as kubeadm
  static pods in `/etc/kubernetes/manifests`
- `etcd` as a kubeadm-managed static pod, local, single member
- `kubelet` as a systemd unit, `kube-proxy` and `CoreDNS` as kubeadm addons
- `containerd` as the CRI runtime with `SystemdCgroup = true`
- the reference CNI plugins `bridge`, `loopback`, `host-local` and `portmap`
  with a hand-written minimal `/etc/cni/net.d` configuration
- the control-plane taint `node-role.kubernetes.io/control-plane:NoSchedule` is
  removed so that the single node also runs workloads

Brunernetes ships **no** patched binaries and **no** vendored control plane. It
ships an installer, a Helm chart, CRDs, a controller manager and a CLI. The
Kubernetes version is pinned as one compatible set (see
[installation.md](../installation.md)); the pin is recorded in
`Togliatti.spec.expectedKubernetesVersion` so the Togliatti controller can
report drift between the installed cluster and the version the installer
produced.

## Alternatives considered

### k3s

k3s is the obvious candidate for a small single-node cluster: one binary,
embedded SQLite or etcd, built-in Flannel, Traefik, ServiceLB and local-path
provisioner, very fast to install.

Rejected because:

- k3s is a *repackaged and modified* distribution. Component flags, the
  data directory (`/var/lib/rancher/k3s`), the kubeconfig location, the
  embedded CNI and the bundled addons all differ from upstream. Every
  diagnostic message Brunernetes emits would have to be written twice: once
  for the k3s reality and once for what upstream documentation says.
- Brunernetes *is* a distribution. Building a distribution on top of another
  distribution makes the value proposition incoherent - the interesting layer
  would be someone else's.
- k3s bundles opinions Brunernetes explicitly rejects (ServiceLB, Traefik
  ingress). Disabling them is possible but means fighting the platform.
- The learning and demonstration value of the project is largely in the
  kubeadm bootstrap path itself.

### minikube

minikube is a development-cluster launcher, not a host installer. It provisions
a VM or a container and runs Kubernetes inside it. Rejected because:

- It adds a virtualisation or container boundary between the host and the
  cluster. The localhost exposure story in
  [ADR 0003](0003-localhost-only-exposure.md) would become "localhost inside
  the minikube VM, then a tunnel", which is exactly the indirection the project
  wants to avoid.
- The specification calls for the cluster to run on the host itself, on the
  host's containerd, with the host's kernel modules and sysctls under the
  installer's control.
- `minikube tunnel` / `minikube service` require a long-running foreground
  process; that is a worse operator experience than a hostPort binding.

### kind (Kubernetes IN Docker)

kind runs nodes as containers. Rejected as the *product* substrate for the same
boundary reason as minikube: the "node" is a container, so host preflight
checks, cgroup configuration, kernel modules and the containerd socket the
project wants to talk about are the *outer* host's, not the node's.

kind is however **kept as CI infrastructure**: the e2e smoke test runs the CRDs,
the Helm chart and the controller against a kind cluster, because that part of
the system is substrate-agnostic. See [testing.md](../testing.md). This is a
deliberate split: kind validates the controller layer, it does not validate the
installer.

### Hand-rolled control plane (static binaries, systemd units, own PKI)

Downloading `kube-apiserver` et al. and wiring systemd units, certificates,
kubeconfigs, RBAC bootstrap, the bootstrap token flow and the addon manifests by
hand.

Rejected because:

- It reimplements kubeadm badly. kubeadm already encodes the certificate
  hierarchy, the kubelet bootstrap flow, component compatibility checks and an
  upgrade path.
- The certificate and PKI surface is the single most error-prone part of a
  Kubernetes install, and getting it wrong quietly produces a cluster that
  works today and fails in one year when a cert expires.
- It would produce a cluster with a non-standard layout, which contradicts the
  reason upstream was chosen in the first place.
- `kubeadm reset` gives a credible uninstall path for free; a hand-rolled
  install would need a hand-rolled teardown that is never tested until it is
  needed.

## Consequences

**Positive**

- Every technical condition the controllers report maps directly to upstream
  Kubernetes documentation, `kubectl` output and existing operator knowledge.
- The blast radius of the project is small: CRDs, one controller manager
  Deployment, one Helm chart, one installer script, one CLI. Removing
  Brunernetes leaves a plain kubeadm cluster; removing the cluster is
  `kubeadm reset`.
- Upgrades follow the upstream kubeadm upgrade procedure. Brunernetes only has
  to move the version pin and re-validate.
- The MVP can be reviewed by any Kubernetes engineer without learning a bespoke
  runtime.

**Negative**

- Installation is slower and heavier than k3s: package repositories must be
  configured, images pulled, preflight checks satisfied.
- The installer must own host-level concerns that a container-based cluster
  hides: swap, cgroup v2, `overlay` and `br_netfilter` modules, sysctls, the
  containerd `SystemdCgroup` setting, port availability. See
  [installation.md](../installation.md).
- Several installer steps require `sudo`. That is a genuine security surface,
  enumerated in [security.md](../security.md).
- A single etcd member on one disk means there is no redundancy. Data loss on
  that disk is cluster loss. This is accepted: the cluster is disposable and the
  desired state lives in version-controlled manifests.
- The node is both control plane and worker, with the taint removed. Workloads
  and the control plane share CPU, memory and the kubelet. A misbehaving
  application can degrade the API server. Mitigations and residual risk are in
  [security.md](../security.md).

## Trade-offs

| Dimension | Chosen (kubeadm + upstream) | k3s | minikube | kind | Hand-rolled |
| --- | --- | --- | --- | --- | --- |
| Fidelity to upstream | Full | Modified | Full inside a VM/container | Full inside containers | Full but non-standard layout |
| Install time and footprint | Moderate | Smallest | Large (VM) | Moderate | Moderate |
| Host-level realism (cgroups, modules, sysctls) | Full | Partial | None (isolated) | None (isolated) | Full |
| Localhost exposure without a tunnel | Yes | Yes | No | No | Yes |
| Maintenance burden on the project | Low (version pin only) | Low | Low | Low | Very high (PKI, units, addons) |
| Teardown story | `kubeadm reset` + explicit cleanup | `k3s-uninstall.sh` | Delete VM | Delete cluster | Bespoke, untested |
| Fits "not a Kubernetes clone" constraint | Yes | Ambiguous | Yes | Yes | Ambiguous |

## Related decisions

- [ADR 0002: CRD and controller boundary](0002-crd-and-controller-boundary.md)
- [ADR 0003: Localhost-only exposure](0003-localhost-only-exposure.md)
- [ADR 0004: Delivery without image build](0004-delivery-without-image-build.md)
- [architecture.md](../architecture.md), [installation.md](../installation.md)
