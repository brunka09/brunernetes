# Security posture

Brunernetes is a single-node development and demonstration platform. Several of
its defining design decisions are, in a production context, straightforward
security defects: one node runs the control plane and the workloads, the
control-plane taint is removed, the installer takes root, and the application is
reachable over plaintext HTTP. These are not accidents and they are not
defended as good practice. They are accepted because the product's stated
boundary is one developer's machine, and the purpose of this document is to
state each one plainly, with its impact, its mitigation and the risk that
remains.

**Scope of the threat model.** In scope: an attacker with a local unprivileged
account on the host; a malicious or compromised container image; an attacker on
the same local network; a supply-chain compromise of a dependency or the
installer's downloads. Out of scope: an attacker who already has root on the
host, and physical access. If either of those holds, the cluster is not the
weakest link.

**Non-negotiable boundary.** Brunernetes must never expose an application
outside the host by default. This is enforced structurally - a ClusterIP Service
plus a `hostIP: 127.0.0.1` `hostPort` binding, never a NodePort, ingress or
LoadBalancer ([ADR 0003](adr/0003-localhost-only-exposure.md)) - and
`AvtoVAZ.spec.policy.allowPublicExposure` defaults to `false`.

Likelihood and impact are rated Low / Medium / High for a single-node developer
host, not for a production cluster.

## Risk register

| # | Risk | Description | Impact | Likelihood | Mitigation | Residual risk |
| --- | --- | --- | --- | --- | --- | --- |
| 1 | Single-node control plane co-located with workloads | `etcd`, `kube-apiserver`, `kube-controller-manager`, `kube-scheduler` and the kubelet run on the same host and the same kernel as every application pod. A container escape reaches the control plane, the cluster PKI in `/etc/kubernetes/pki` and the entire etcd datastore. | High | Low | The architecture is the product ([ADR 0001](adr/0001-single-node-upstream-kubernetes.md)). Workloads run non-privileged with `allowPrivilegeEscalation: false` and all capabilities dropped. Only one application is permitted (`maxApplications: 1`). Documented as unsuitable for production and for untrusted images. | High, accepted. There is no separation to recover. Do not run images you would not run as a local process. |
| 2 | Control-plane taint removed | Step 7 of the installer runs `kubectl taint nodes --all node-role.kubernetes.io/control-plane-` so workloads can schedule. This deliberately removes the only scheduling barrier between application pods and the control plane. | High | High (it always happens) | Unavoidable on one node - without it every workload stays `Pending`. The action is explicit, logged, and documented here and in [installation.md](installation.md) rather than performed silently. Resource limits on the workload reduce the chance of starving control-plane components. | High, accepted. A workload that exhausts CPU or memory can destabilise the API server. |
| 3 | hostPort binding on the node network namespace | The pod binds `127.0.0.1:30080` on the host through the CNI portmap plugin. Any local process or local user can reach it, and the port is a shared host resource that two applications cannot both hold. | Medium | Medium | `hostIP` is pinned to `127.0.0.1`, so the binding is not reachable off-host even on a machine with a public address. `allowPublicExposure: false` is enforced by policy. `maxApplications: 1` avoids port contention in v0.1. | Medium. Every local user of the host can reach the application. A `hostIP` misconfiguration in a future change would silently make it public; this must stay a reviewed field. |
| 4 | No TLS on the loopback endpoint | Traffic to `http://127.0.0.1:30080` is plaintext. | Low | High | The traffic never leaves the loopback interface, so there is no network path to intercept it. Anything that could read loopback traffic already has the privilege to read the process memory behind it. | Low, accepted for v0.1. The real cost is pedagogical: the demo normalises plaintext. Optional TLS is scheduled for v0.3 ([roadmap.md](roadmap.md)). Do not send credentials through the demo endpoint. |
| 5 | Controller RBAC scope | The `brunernetes-manager` `ClusterRole` grants full CRUD plus watch on `apps/deployments` and core `services` cluster-wide, read on `pods`, `create`/`patch` on `events`, full CRUD on `brunerapps` with `status` and `finalizers`, and read on `avtovazs` with `avtovazs/status`. Compromising the controller pod yields the ability to create or replace any Deployment or Service in any namespace - including in `kube-system`. | High | Low | The permission set is narrow by verb-resource pair rather than wildcarded: no `secrets`, no `nodes` write, no `clusterroles`, no `*` resource, and no rules at all for `togliattis` or `konveers` because no controller reconciles them yet. The controller runs one workload in one namespace and its image is built from this repository. | Medium. Cluster-wide Deployment and Service write is wider than the controller needs. Scoping it to a namespaced `Role` where possible is a v0.3 item. |
| 6 | Controller runs without `runAsNonRoot` | The built workload pod security context sets `allowPrivilegeEscalation: false`, `privileged: false`, drops `ALL` capabilities and applies the `RuntimeDefault` seccomp profile, but does not set `runAsNonRoot` or a `runAsUser`. A container whose image defaults to UID 0 runs as root inside the container. | Medium | Medium | Privilege escalation is blocked and every capability is dropped, so container root is substantially defanged. The demo image serves as an unprivileged nginx on port 8080. | Medium. Combined with risk 1, container root on a node that is also the control plane is a shorter path to the host than it should be. Adding `runAsNonRoot: true` is a v0.3 item; it is a behavioural change because it breaks images that need UID 0. |
| 7 | containerd socket | `/run/containerd/containerd.sock` grants full control of the container runtime - creating a privileged container with the host filesystem mounted is equivalent to root. | High | Low | The socket is root-owned with restrictive permissions and is **never** mounted into any pod Brunernetes creates. The controller talks to the API server, never to the runtime. The installer references the socket path only in the kubeadm CRI socket setting and in `kubeadm reset`. | Low, provided no future feature mounts it. Any proposal to give a controller runtime access should be rejected at review. |
| 8 | Admin kubeconfig at `/etc/kubernetes/admin.conf` | A cluster-admin credential with no expiry short of the certificate lifetime. The installer copies it to `$HOME/.kube/config` and chowns it to the invoking user. | High | Medium | `kubeadm` writes it mode `0600` root-owned; the copy is owned by the invoking user. Any local user who reads either file is cluster-admin, which on this architecture means host-adjacent root. | Medium. This is the standard kubeadm posture, inherited rather than chosen. On a shared host, do not copy it into a home directory; use `KUBECONFIG=/etc/kubernetes/admin.conf` under `sudo`. Per-user certificates are out of scope. |
| 9 | Installer runs as root | The installer loads kernel modules, sets sysctls, installs packages, rewrites `/etc/containerd/config.toml`, runs `kubeadm init`, writes `/opt/cni/bin` and `/etc/cni/net.d`, and restarts system services. A bug or a compromised binary runs with full host privilege. | High | Low | Preflight is a pure function that runs before any change and aborts on the first blocking failure. `--dry-run` prints every command and file write without executing them, and relaxes only the root check. Every privileged operation is enumerated with its justification in [installation.md](installation.md). Every write path is a constant in `versions.go`, not a value derived from user input. | Medium. There is no way to install a Kubernetes control plane without root. The mitigation is reviewability, not privilege reduction. Always run `--dry-run` first on a host you care about. |
| 10 | Uninstall flushes all iptables rules | Uninstall step 4 runs `iptables -F`, `-t nat -F`, `-t mangle -F` and `-X`, which removes every rule in those tables - not only the ones Kubernetes added. | Medium | Medium | The flush runs only during an explicit `brunerctl uninstall`, and only after `kubeadm reset`. Documented in [installation.md](installation.md). | Medium. On a host with its own firewall rules this is destructive and silently disables them until the firewall service is restarted. Prefer a reboot over the flush on such hosts. |
| 11 | Mutable `:latest` image tags | A `:latest` reference means the running container's contents are not determined by the manifest. A pull on any restart can change the workload, and the same manifest is not reproducible. | Medium | High | `AvtoVAZ.spec.policy.latestTagPolicy` supports `Allow`, `Warn` and `Reject`. All installer presets set `Warn`, which surfaces `PolicyWarning=True` with a reason and an event, so the condition is visible without blocking a demo. `Reject` is available for anyone who wants it enforced. | Medium, deliberately. `Warn` is a usability choice; policy is advisory in the MVP ([ADR 0002](adr/0002-crd-and-controller-boundary.md)). Making `Reject` the `strict` default is a v0.3 item. |
| 12 | Policy is advisory, not enforcing | There is no admission webhook. Policy is evaluated inside the controller after the object is admitted, so a non-compliant `BrunerApp` is stored and reconciled, with the violation surfaced as a condition and an event. | Medium | High | Deliberate ([ADR 0002](adr/0002-crd-and-controller-boundary.md)): a webhook is a second serving path with its own certificate lifecycle and a failure mode that can block the API server. CRD OpenAPI validation still rejects structurally invalid objects at admission. Violations are always visible in `brunerctl status` and in events. | Medium, accepted. Policy is a guardrail for a cooperative user, not a control against an adversary. Anyone with write access to a `BrunerApp` can violate policy and get a running workload. |
| 13 | Supply chain: installer downloads | The installer adds a package repository and signing key from `pkgs.k8s.io`, installs distribution packages, and downloads `cni-plugins-linux-amd64-v1.6.2.tgz` from GitHub releases directly into `/opt/cni/bin`. | High | Low | Package installation is GPG-verified through apt against the official Kubernetes key, and versions are pinned to `1.32.3-1.1` and held with `apt-mark hold`. All downloads use HTTPS with certificate validation. The CNI plugin version is pinned. | Medium. The CNI tarball is **not** checksum-verified - only TLS and the pinned URL protect it. Checksum verification is a v0.3 item and is the highest-value single fix in this table. |
| 14 | Supply chain: controller image and dependencies | The controller image is referenced by tag, not digest, and the Go module graph pulls upstream dependencies. | Medium | Low | Dependencies are pinned in `go.mod` and `go.sum` with checksum verification. The image is built from this repository. CI runs `go vet` and `golangci-lint`. | Medium. No digest pinning, no image signing, no SBOM. All three are v0.3 items. |
| 15 | Secrets handling | `BrunerApp` has no field for secrets, environment variables from secrets, or mounted volumes. Nothing in the API carries credential material. | Low | Low | The absence is structural rather than enforced: there is no field to misuse. The controller has no RBAC rule for `secrets` and cannot read them. Image pull secrets are equally absent, which restricts the platform to public or pre-pulled images ([ADR 0004](adr/0004-delivery-without-image-build.md)). | Low. If secret support is added later, the RBAC exclusion and the API surface must be reconsidered together, not separately. |
| 16 | Demo workload exposure | `demo/factory-showroom` is a static site served by nginx on port 8080 with a `/healthz` endpoint, reachable at `127.0.0.1:30080`. | Low | Low | No backend, no database, no state, no authentication surface, no user input processed server-side. A local-only static file server. | Low. |
| 17 | Generated kubeadm configuration | `/etc/brunernetes/kubeadm.yaml` records the cluster topology and the optional API advertise address. | Low | Low | Written mode `0600`, root-owned, under `/etc/brunernetes`. It contains no keys or tokens - the PKI lives under `/etc/kubernetes/pki`, managed by kubeadm. | Low. |
| 18 | Node label and factory identity | The node carries `bruner.dev/factory=togliatti`. | Low | Low | A descriptive label with no authorization meaning; no RBAC rule or policy decision depends on it. | Low. Do not make it a security boundary later without revisiting this entry. |

## Highest-value improvements

In order of the ratio between risk reduced and effort required:

1. **Verify the CNI plugin tarball checksum** (risk 13). A constant and four
   lines of code close an unauthenticated code path into `/opt/cni/bin`.
2. **Add `runAsNonRoot: true` to the workload security context** (risk 6). A
   behavioural change, so it needs a release note, but it removes container root
   from a node that is also the control plane.
3. **Pin the controller image by digest** (risk 14).
4. **Narrow the controller `ClusterRole`** to a namespaced `Role` for Deployment
   and Service management (risk 5).
5. **Make the uninstall iptables flush opt-in** (risk 10).

All five are scheduled in [roadmap.md](roadmap.md) v0.3.

## Deployment guidance

Brunernetes is appropriate for a developer workstation, a disposable VM, a
workshop machine or a demonstration laptop.

It is not appropriate for a shared multi-user host, a machine with a public
network interface and a permissive firewall, a machine holding production
credentials, or a host running untrusted images. There is no supported
configuration that makes it appropriate for those, and none is planned
([roadmap.md](roadmap.md) non-goals).

## Related documents

- [ADR 0001: Single node on upstream Kubernetes](adr/0001-single-node-upstream-kubernetes.md)
- [ADR 0002: CRD and controller boundary](adr/0002-crd-and-controller-boundary.md)
- [ADR 0003: Localhost-only exposure](adr/0003-localhost-only-exposure.md)
- [ADR 0004: Delivery without image build](adr/0004-delivery-without-image-build.md)
- [installation.md](installation.md) - the `sudo` table and the dry-run behaviour
- [architecture.md](architecture.md) - trust boundaries in prose
- [roadmap.md](roadmap.md) - when each mitigation lands
