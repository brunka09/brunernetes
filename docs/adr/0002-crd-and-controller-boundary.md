# ADR 0002: Four CRDs, their scopes, and no admission webhook in the MVP

- Status: Accepted
- Date: 2026-09-07
- Deciders: Brunernetes maintainers
- Applies to: v0.1 (MVP)

## Context

Brunernetes adds an opinionated API on top of an ordinary kubeadm cluster
(see [ADR 0001](0001-single-node-upstream-kubernetes.md)). Four concerns have to
be modelled:

1. **The environment.** Is the node ready, is the API server healthy, is
   containerd up, is the pod network working, how much CPU and memory is there,
   can this node publish a loopback endpoint at all?
2. **The rules.** What operating mode do applications inherit, are readiness
   probes and resource requests mandatory, what happens to a `:latest` tag, how
   many applications may exist, which registries are allowed?
3. **The product.** One web application: its image, replicas, port, probes,
   resources, loopback exposure - and the Deployment and Service that implement
   it.
4. **The delivery of a new image of that product.** Validate, apply, wait for
   rollout, wait for readiness, verify the endpoint.

These are four different lifecycles with four different cardinalities and four
different audiences. Conflating them into one giant `Cluster` CRD would produce
a resource whose status mixes node health with HTTP verification results, and
whose spec cannot be edited by two actors without conflict.

Kubebuilder and controller-runtime give a webhook server essentially for free,
so the second question is whether policy - "strict profile requires a readiness
probe", "`:latest` is rejected", "at most one application" - should be enforced
by a validating admission webhook or by the controllers.

## Decision

### Four kinds with these scopes

| Kind | Scope | Cardinality (MVP) | Owns | Rationale |
| --- | --- | --- | --- | --- |
| `Togliatti` | Cluster | 1, conventionally named `togliatti` | nothing | Describes the node, the API server, the runtime and the pod network. These are cluster-wide facts. A namespaced resource describing the node would be a lie about ownership and would allow N contradictory copies. |
| `AvtoVAZ` | Cluster | 1, conventionally named `avtovaz` | nothing | Factory-wide configuration and policy. Policy that can be overridden per namespace is not policy. `spec.factoryRef` (default `"togliatti"`) binds it to the environment resource. |
| `Konveer` | Namespaced | N, short lived | nothing (mutates a `BrunerApp`) | A delivery workflow is a unit of work, like a Job. It is created, it runs, it reaches a terminal phase, it is garbage collected or kept for audit. It targets a `BrunerApp` in the **same namespace** via `spec.appRef`, so its blast radius is bounded by a namespace and by RBAC on that namespace. |
| `BrunerApp` | Namespaced | 1 in the MVP, enforced by `AvtoVAZ.spec.policy.maxApplications` | one `Deployment`, one `Service` | The workload it produces is namespaced, so the resource that owns it must be namespaced too - owner references cannot cross from a cluster-scoped owner to a namespaced dependent without breaking garbage collection semantics. |

The cluster-scoped pair describes *the factory*: facts and rules that are true
exactly once per cluster. The namespaced pair describes *work*: a product and
the deliveries of that product. That split is the whole boundary.

`Konveer` deliberately does **not** own the `BrunerApp` it delivers to. It
applies changes to it (image, and the optional `spec.appTemplate` fields) and
then observes it. If `Konveer` owned the `BrunerApp`, deleting a finished
delivery record would delete the running application. Ownership stops at the
`BrunerApp` → `Deployment`/`Service` edge.

### No admission webhook in the MVP

Policy is evaluated **inside the controllers** and surfaced as status conditions
and events. Structural constraints that CRD schema validation can express
(enums, `Minimum`/`Maximum`, `Pattern`, `MinLength`/`MaxLength`, defaults) are
expressed in the OpenAPI schema generated from the kubebuilder markers in
`api/v1alpha1` and are therefore enforced by the API server at admission time
without any webhook. Everything that requires reading *another* object -
the `AvtoVAZ` policy, the current application count, the node's exposure
capability - is evaluated during reconciliation.

Concretely:

- Schema-level (API server, no webhook): `spec.image` non-empty and ≤ 512
  characters; `replicas` in 0..5; `port` in 1..65535; `healthPath` matching
  `^/.*$`; `localExposure.hostIP` matching `^127\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}$`;
  `hostPort` in 1024..65535; `profile` in `garage|showroom|strict|friday`;
  `qualityGate` in `practical|strict`; `latestTagPolicy` in
  `Ignore|Warn|Reject`; `maxApplications` fixed at 1; `allowPublicExposure`
  restricted by `+kubebuilder:validation:Enum=false`.
- Controller-level: mode merging across `AvtoVAZ.spec.defaultMode` and
  `BrunerApp.spec.mode`; "strict requires a readiness probe and resource
  requests"; `:latest` handling under `latestTagPolicy`; allowed-registry
  matching; the `friday` profile's override requirement; the application count
  against `maxApplications`.

A rejected policy check sets `Degraded=True` with reason `PolicyViolation` (or
`OverrideRequired`, or `CapacityExceeded`), emits a `Warning` event, and leaves
the workload untouched. A non-blocking deviation sets `PolicyWarning=True` with
reason `PolicyDeviation` and emits a `Warning` event while the rollout
continues. Reason constants live in `api/v1alpha1/common_types.go`; the exact
mapping is documented in [reconciliation.md](../reconciliation.md).

## Alternatives considered

### One CRD for everything (`Factory`)

A single cluster-scoped resource with sections for environment, policy, the app
and deliveries. Rejected: unrelated write paths collide on one `spec`, status
becomes a grab bag, RBAC cannot separate "may change policy" from "may deploy",
and a namespaced Deployment cannot be owned cleanly by a cluster-scoped object.

### Making `AvtoVAZ` namespaced

Would allow per-namespace policy. Rejected for the MVP: the MVP has exactly one
application and `maxApplications` is capped at 1, so per-namespace policy has no
consumer, and a namespaced policy resource makes "the factory rules" ambiguous
when two namespaces disagree. Revisit if multi-app support lands (see
[roadmap.md](../roadmap.md) and [open-questions.md](../open-questions.md)).

### Making `Konveer` cluster-scoped

Would match `Togliatti`/`AvtoVAZ`. Rejected: a delivery mutates a namespaced
`BrunerApp`, so a cluster-scoped `Konveer` would need cluster-wide write access
to every `BrunerApp` in every namespace, and RBAC on deliveries could no longer
be delegated per namespace.

### A validating (and/or mutating) admission webhook in the MVP

Rejected for v0.1. Considered again for v0.3 (see [roadmap.md](../roadmap.md)).

## Consequences

**Positive**

- No webhook server means no TLS serving certificate to issue, rotate and
  distribute, no cert-manager dependency or self-signed CA bootstrap, no
  `caBundle` injection in the Helm chart, and no `failurePolicy` decision.
- No webhook means no availability coupling. On a single node
  ([ADR 0001](0001-single-node-upstream-kubernetes.md)) a webhook with
  `failurePolicy: Fail` that is itself down makes the resources it guards
  unwritable - including, in the worst configuration, the resources needed to
  fix it. A restart of the controller manager is then a cluster outage.
- Policy decisions are observable after the fact. `kubectl get brunerapp`,
  `kubectl describe`, `brunerctl status` and `brunerctl events` all show *why*
  something did not roll out, with a condition, a machine-readable reason and a
  human message. A webhook rejection only exists in the client's terminal at
  write time and is lost afterwards.
- Policy is testable with envtest and plain unit tests over deterministic
  functions (`MergeModes`, `ValidateMode`, `ParseImageReference`,
  `RegistryAllowed`, `IsLoopbackHostIP`) without standing up a webhook server.

**Negative**

- Invalid-by-policy objects are **accepted** by the API server and persisted.
  `kubectl apply` succeeds and the failure appears asynchronously in status.
  Users must be taught to read status, and `brunerctl` must make that easy -
  `brunerctl app deploy` waits for and prints the resulting conditions rather
  than returning as soon as the write succeeds.
- There is a window between the write and the next reconcile in which the
  cluster contains a resource that policy forbids.
- Policy is enforced only for objects the controllers reconcile. A user who
  bypasses `BrunerApp` and creates a raw `Deployment` is not subject to factory
  policy at all. This is accepted: Brunernetes is a developer experience layer,
  not a multi-tenant guard rail. See [security.md](../security.md).
- Defaulting that cannot be expressed as a CRD `+kubebuilder:default` (for
  example, the merged effective mode) must be computed on every reconcile and
  reported in `status.effectiveMode` instead of being written back into the
  spec. The spec therefore does not show the values that are actually in force;
  status does.

## Trade-offs

| Concern | Controller-evaluated policy (chosen) | Admission webhook |
| --- | --- | --- |
| Feedback latency | Asynchronous, one reconcile | Synchronous, at write time |
| Feedback durability | Persisted in conditions and events | Transient, client-side only |
| Availability coupling | None | Webhook outage can block writes |
| Operational cost | None beyond the controller | TLS certs, rotation, `caBundle`, `failurePolicy` |
| Cross-object policy (count, node capability) | Natural | Requires a live client and careful caching |
| Prevents bad state from being persisted | No | Yes |
| Testability | Unit + envtest | Needs a webhook harness |

The decisive factor is availability coupling on a single node with no redundancy
combined with the fact that the MVP's policy is advisory in nature: it guards
one demo application on one developer machine, not a shared production estate.

## Related decisions

- [ADR 0001: Single node on upstream Kubernetes](0001-single-node-upstream-kubernetes.md)
- [ADR 0003: Localhost-only exposure](0003-localhost-only-exposure.md)
- [ADR 0004: Delivery without image build](0004-delivery-without-image-build.md)
- [api.md](../api.md), [reconciliation.md](../reconciliation.md)
