# ADR 0003: Localhost-only exposure via ClusterIP plus a loopback hostPort

- Status: Accepted
- Date: 2026-09-07
- Deciders: Brunernetes maintainers
- Applies to: v0.1 (MVP)

## Context

The single application Brunernetes runs must be reachable from a browser on the
host, and from nowhere else. The specification states it plainly: no external
ingress controller, no public exposure, demo application reachable at localhost
only. `AvtoVAZ.spec.policy.allowPublicExposure` is typed with
`+kubebuilder:validation:Enum=false`, which makes the guarantee part of the API
schema rather than a convention - the API server rejects any attempt to set it
to `true`.

The host is a developer laptop, a VM or a small cloud instance. On a cloud
instance the network interface has a routable address and, frequently, a
permissive default security group. Any exposure mechanism that binds `0.0.0.0`
therefore publishes the application to whatever the host's firewall happens to
allow - which is not a decision the Brunernetes installer is in a position to
make on the user's behalf.

The cluster is a single node (see
[ADR 0001](0001-single-node-upstream-kubernetes.md)) using the reference CNI
plugins with a bridge network. There is no cloud controller manager, so there is
no `LoadBalancer` implementation.

## Decision

Every `BrunerApp` gets:

1. **A `ClusterIP` Service, always.** This is the stable in-cluster address, it
   is what `status.clusterIPEndpoint` reports, and it is what the pod network
   and CoreDNS resolve. It is created unconditionally, independent of local
   exposure settings.
2. **A loopback `hostPort` on the application container, when
   `spec.localExposure.enabled` is `true` (the default).** The container port is
   published with `hostIP` bound to a loopback address and `hostPort` set to the
   configured port. Defaults from `api/v1alpha1/common_types.go`:
   `enabled: true`, `hostIP: "127.0.0.1"`, `hostPort: 30080`. `hostIP` is
   constrained by the pattern `^127\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}$`, and
   the helper `IsLoopbackHostIP` re-checks the prefix `127.` in code.
   `hostPort` is constrained to 1024..65535 so that no privileged port can be
   requested.

The hostPort binding is implemented by the **`portmap` CNI plugin**, chained
after `bridge` in `/etc/cni/net.d`. `portmap` reads the `portMappings`
capability argument that the kubelet passes for each container port carrying a
`hostPort`, and installs the corresponding DNAT rules. Without `portmap` in the
chain, `hostPort` is silently ignored - the pod starts, the Service works, and
nothing answers on `127.0.0.1:30080`. That is why the CNI configuration in
[installation.md](../installation.md) lists all four plugins
(`bridge`, `loopback`, `host-local`, `portmap`) and why `Togliatti` reports
`status.localEndpointCapable` and the `LocalEndpointCapable` condition.

`BrunerApp.status.localEndpoint` reports the resulting URL, for example
`http://127.0.0.1:30080`. `Konveer.spec.verify` performs an HTTP GET against
that endpoint after readiness and records the answering endpoint in
`status.verifiedEndpoint`.

## Alternatives considered

### `NodePort`

The conventional single-node answer. Rejected:

- `kube-proxy` programs NodePort rules on **all** node addresses. There is no
  supported per-Service way to restrict a NodePort to loopback; the
  `--nodeport-addresses` kube-proxy flag is cluster-wide, and setting it to
  `127.0.0.0/8` is both an unusual configuration and a change to an upstream
  component's operating parameters that would apply to every Service in the
  cluster.
- On a cloud instance, a NodePort is reachable from outside the host the moment
  the security group allows the port. The failure is silent and the user finds
  out from a log.
- The port range (default 30000–32767) is fixed and is a poorer fit than an
  arbitrary user-chosen port.

### Ingress controller (nginx, Traefik, ...)

Rejected: it is an explicit non-goal. It adds a Deployment, a Service, an
`IngressClass`, RBAC, TLS material and a controller that must itself be exposed
somehow - which reduces to the same problem one layer up, plus more moving
parts on a 2 vCPU / 4 GB host, plus a hostname resolution story (`/etc/hosts`
edits) for a single page.

### `LoadBalancer`

Rejected: there is no cloud controller manager and no MetalLB, so the Service
stays `Pending` forever. Installing MetalLB to hand out an address on the host's
LAN is the opposite of the requirement.

### `kubectl port-forward` / `brunerctl` tunnel

Rejected as the primary mechanism: it requires a long-lived foreground process,
dies with the terminal, does not survive pod restarts without reconnect logic,
and cannot be reported as a stable `status.localEndpoint`. It remains a useful
fallback documented in [installation.md](../installation.md) when
`portmap` is unavailable.

### `hostNetwork: true` on the pod

Rejected: it puts the container directly in the host network namespace, which
means it binds whatever address the process binds - typically `0.0.0.0` - and
removes network isolation entirely. It also makes port conflicts with host
services immediate and unrecoverable.

## Consequences

**Positive**

- The default binding is `127.0.0.1`. Traffic from off-host cannot reach the
  application even if the host firewall is wide open, because nothing is
  listening on a routable address.
- No additional workloads: no ingress controller, no load balancer speaker, no
  extra CPU or memory on a small host.
- `http://127.0.0.1:30080` is a stable, bookmarkable, restart-surviving URL that
  fits in a status field and in CLI output.
- The guarantee is auditable in three independent places: the CRD schema
  (`allowPublicExposure` enum, `hostIP` pattern), the controller code
  (`IsLoopbackHostIP`), and the generated pod spec.

**Negative**

- `hostPort` is a scheduling constraint: the port must be free on the node, and
  only one pod at a time can hold a given `hostIP:hostPort` pair. With
  `replicas > 1` and the same hostPort, additional replicas cannot be scheduled.
  On a single node this makes `spec.replicas: 1` the only sensible value when
  local exposure is enabled, even though the schema permits up to 5.
- A surge-style rolling update would need two pods holding the same hostPort
  simultaneously. This is the direct reason the only supported rollout strategy
  is `steady` (`Rollout` enum has exactly one value, `steady`): a surge-free
  rolling update that terminates the old pod before starting the new one. The
  cost is a short gap in availability during an image change, which is
  acceptable for a local demo and is stated to the user rather than hidden.
- Publishing on loopback means it is genuinely local. Demonstrating the
  application to someone on another machine requires an explicit, user-owned
  tunnel (SSH `-L`, or `kubectl port-forward --address`). Brunernetes will not
  do this for the user.
- `hostPort` is widely discouraged in shared clusters. Brunernetes uses it
  deliberately and in a bounded way; that reasoning has to be repeated wherever
  the pattern is copied.

## Failure modes

| Failure mode | Symptom | Detection | Response |
| --- | --- | --- | --- |
| `portmap` missing from the CNI chain | Pod Running and Ready, Service works, `curl 127.0.0.1:30080` gives connection refused | Installer preflight checks `/opt/cni/bin/portmap` and the `plugins` list in `/etc/cni/net.d`; `Togliatti` sets `LocalEndpointCapable=False` | Installer refuses to proceed or repairs the CNI configuration; `BrunerApp` sets `PolicyWarning`/`Degraded` with a message stating that the loopback publication is not available |
| Host port already bound by another process (for example a local dev server on 30080) | Pod stuck `Pending` or the sandbox fails to start with a port allocation error | `kubectl describe pod` scheduling/sandbox events; `BrunerApp` `Degraded=True` | Choose a different `spec.localExposure.hostPort`; the installer preflight lists the ports it needs |
| `replicas > 1` with local exposure enabled | One pod Ready, the rest `Pending` with no node available | `status.replicas` > `status.readyReplicas` persistently, `QualityControl` stage | Reduce to one replica or disable local exposure; the controller reports the conflict explicitly |
| Rolling update overlap | Brief connection refused on the loopback port during an image change | Expected and documented | `steady` rollout minimises the window; `Konveer` verification retries within `spec.timeouts.readinessSeconds` |
| `hostIP` set to a non-loopback address | Write rejected by the API server | CRD pattern validation; `IsLoopbackHostIP` as a second line of defence | None needed - the object never persists |
| IPv6-only client (`localhost` resolving to `::1`) | Browser cannot connect while `curl 127.0.0.1` works | Manual | Use the literal `127.0.0.1`. IPv6 loopback publication is out of scope for the MVP; see [open-questions.md](../open-questions.md) |
| Host firewall (`iptables`/`nftables`) rules flushed by another tool | Loopback publication stops working until the CNI chain is reprogrammed | Endpoint verification failure; `Konveer` `Verified=False`, reason `EndpointUnverified` | Restart the pod so the CNI plugin reprograms the rules; report the technical condition in the status message |
| Controller cannot reach the endpoint although the endpoint works | Verification inconclusive (the controller pod is not in the host network namespace) | By design | The verification stage is **skipped**, not failed: `KonveerVerify.enabled` documents this, and the stage state is `Skipped` |

## Trade-offs

| Option | Off-host reachable by default | Extra workloads | Stable URL | Survives restarts | Scheduling constraint |
| --- | --- | --- | --- | --- | --- |
| ClusterIP + loopback hostPort (chosen) | No | None | Yes | Yes | Yes (one pod per host port) |
| NodePort | Yes | None | Yes | Yes | No |
| Ingress controller | Yes | Controller + Service | Yes (needs hostname) | Yes | No |
| LoadBalancer | Yes | Speaker/CCM | Yes | Yes | No |
| `kubectl port-forward` | No | None | No | No | No |
| `hostNetwork` | Yes | None | Yes | Yes | Severe |

## Related decisions

- [ADR 0001: Single node on upstream Kubernetes](0001-single-node-upstream-kubernetes.md)
- [ADR 0002: CRD and controller boundary](0002-crd-and-controller-boundary.md)
- [architecture.md](../architecture.md), [installation.md](../installation.md),
  [security.md](../security.md)
