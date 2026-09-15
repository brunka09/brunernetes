# Reconciliation model

This document describes what each Brunernetes controller watches, what it owns,
when it runs, what it writes, and how it fails. Field names, condition types and
reason strings follow [api.md](api.md) and the Go types in `api/v1alpha1`.

## Implementation status

| Controller | Status in v0.1 (MVP) | Source |
| --- | --- | --- |
| **BrunerApp** | **Implemented** | `internal/controller/brunerapp_controller.go`, with the deterministic build and policy logic in `internal/factory` |
| Togliatti | **No reconciler.** Deferred to v0.2; `brunerctl factory status` reads and reports the environment directly | `internal/cli/status.go` |
| AvtoVAZ | **No reconciler.** Deferred to v0.2; consumed as configuration by the `BrunerApp` controller and reported by `brunerctl` | `internal/cli/status.go` |
| Konveer | **No reconciler.** Deferred to v0.2; `brunerctl konveer apply` runs the four delivery stages client-side and writes the result into `Konveer.status` | `internal/cli/konveer.go` |

All four CRDs exist and are installed by the Helm chart in v0.1, so a user can
create a `Togliatti`, an `AvtoVAZ` or a `Konveer` today. What differs is who
acts on them:

- `AvtoVAZ` is fully honoured, because policy and default mode are evaluated
  inside the `BrunerApp` reconciler, which is where policy belongs under
  [ADR 0002](adr/0002-crd-and-controller-boundary.md). Nothing writes
  `AvtoVAZ.status`.
- `Togliatti` is a declaration of the expected environment. Nothing writes its
  status; `brunerctl status` and `brunerctl factory status` inspect the node,
  the runtime and the control-plane pods live and report the same facts the
  future controller will persist.
- `Konveer` is **not** inert. `brunerctl konveer apply` stores the resource as
  the record of the delivery and then executes `Validate -> Apply -> Rollout ->
  Verify` itself, writing `status.phase`, `status.stages`,
  `status.appliedImage`, `status.verifiedEndpoint`, `status.startTime`,
  `status.completionTime` and the `Validated`, `RolledOut`, `Verified` and
  `Succeeded` conditions. A `Konveer` created with `kubectl apply` alone does
  nothing, because no controller is watching.

This is a deliberate MVP boundary, not an oversight. The resource contract is
the same in both arrangements: moving the stages behind a controller in v0.2
changes who runs them, not what a `Konveer` means. The sections below marked
*(deferred)* are the specification the v0.2 implementation must satisfy.

The practical consequence of a client-side workflow is worth stating plainly: a
delivery lives and dies with the `brunerctl` process. Interrupt the command and
the `Konveer` is left at whatever phase it had reached, with no controller to
resume it. Rerun the command to drive it forward.

## Shared rules

These rules apply to every controller, implemented or deferred.

**Idempotency.** Every controller is a pure function of observed state plus
spec. Running `Reconcile` twice with the same inputs must produce the same
objects and the same status, and the second run must not write anything. The
implemented `BrunerApp` controller enforces this in two places: the desired
objects are built by deterministic pure functions in `internal/factory`, and
`equalStatus` compares the computed status against the stored one - ignoring
condition timestamps - so an unchanged status produces no API write on a
periodic resync.

**Owner references.** Every object a controller creates carries a controller
owner reference to the custom resource that produced it, set with
`controllerutil.SetControllerReference`. Deletion is therefore handled by
Kubernetes garbage collection; no finalizer is used in the MVP, and the
reconciler returns immediately when `metadata.deletionTimestamp` is set.

**Immutable fields.** A Deployment's `spec.selector` and a Service's
`spec.clusterIP` and `spec.type` cannot be changed after creation. The
`BrunerApp` controller writes the selector only when the object is being
created (`CreationTimestamp.IsZero()`) and always forces
`spec.type: ClusterIP` without touching the assigned cluster IP. Selector
labels are therefore restricted to values that never change for a given
application: `app.kubernetes.io/name`, `app.kubernetes.io/instance` and
`app.kubernetes.io/part-of`.

**Labels and annotations are merged, not replaced.** `mergeStringMaps` copies
the desired keys over the existing map so labels added by other tooling survive
reconciliation.

**Conflict handling.** A `409 Conflict` on a status update is not an error: the
controller returns `Requeue: true` and reconciles again against fresh state.
Every other error is returned to controller-runtime, which applies its default
exponential backoff.

**Message discipline.** Every condition message and every event message states
the technical condition first and the themed wording second. Messages are
truncated at 32 KiB (`truncateMessage`) to stay inside the API server limit.

**Requeue cadence.** Derived from the effective shift:

| Situation | Interval | Constant |
| --- | --- | --- |
| Steady state, `shift: day` | 30 s | `requeueDayShift` |
| Steady state, `shift: night` | 2 min | `requeueNightShift` |
| A fault or a policy violation is blocking | 15 s | `requeueOnFault` |

---

## BrunerApp controller - implemented

Reconciles one `BrunerApp` into one `Deployment` and one `Service`, and
publishes the application on the loopback interface.

### Watched and owned

| Relationship | Resource | Notes |
| --- | --- | --- |
| `For` | `bruner.dev/v1alpha1 BrunerApp` | The primary resource |
| `Owns` | `apps/v1 Deployment` | Named after the `BrunerApp`, in its namespace |
| `Owns` | `v1 Service` | ClusterIP, named after the `BrunerApp` |
| Read | `bruner.dev/v1alpha1 AvtoVAZ` (cluster-scoped) | Read on every reconcile via `List`; **not** watched, so a policy change is picked up on the next periodic resync rather than immediately |
| Read | `v1 Pod` | Listed by selector labels to explain a stalled rollout |
| Write | `v1 Event` | Via the manager's event recorder, `brunerapp-controller` |

Controller name: `brunerapp`.

### Triggers

1. Create, update or delete of a `BrunerApp`.
2. Any change to the owned `Deployment` or `Service`, including status changes
   propagated by `kube-controller-manager` - this is what makes replica and
   readiness transitions visible without polling.
3. The periodic requeue described above.

A `BrunerApp` that is being deleted is skipped: owner references remove the
Deployment and the Service, so there is nothing to clean up.

### Reconcile sequence

1. **Fetch.** `Get` the `BrunerApp`. `NotFound` is ignored.
2. **Resolve the mode.** `List` all `AvtoVAZ` resources. Select the one named
   `avtovaz`, otherwise the alphabetically first; when the list is empty or the
   read fails, fall back to `factory.DefaultPolicy()` with no factory mode, so
   a cluster without an `AvtoVAZ` still works. Merge
   `AvtoVAZ.spec.defaultMode` with `BrunerApp.spec.mode` through `MergeModes`,
   then `ValidateMode`. An invalid mode is a terminal fault for this pass.
3. **Mark reconciling.** Set `Reconciling=True`, reason `Reconciling`, message
   "Desired state is being applied. Work order accepted at the gate."
4. **Evaluate policy** with `factory.EvaluateApp` *before writing anything to
   the cluster*, in this order: image reference parsing, mutable tag against
   `latestTagPolicy`, allowed registries, readiness probe requirement, resource
   request requirement, the `friday` override guard, and a non-loopback
   `hostIP`. Warnings become `PolicyWarning=True`; the first violation blocks.
5. **Apply the Deployment** with `CreateOrUpdate`.
6. **Apply the Service** with `CreateOrUpdate`.
7. **Observe.** Read the Deployment status and list the pods matching the
   selector labels to collect scheduling blocks, image pull failures, crash
   loops, readiness failures and progress deadline exhaustion into a
   `factory.WorkloadState`.
8. **Assess.** `factory.AssessWorkload` turns that state into a single verdict -
   stage, available, degraded, reason, message - checking in the order a human
   would debug: scheduling, image, crash loop, progress deadline, readiness,
   replica count.
9. **Write status** and requeue.

### Conditions set

| Condition | `True` when | `False` when | Reasons used |
| --- | --- | --- | --- |
| `Reconciling` | The desired state is still converging | The observed state matches the desired state, or reconciliation is paused by a policy violation or a fault | `Reconciling`, the assessment reason, the violation reason, `ValidationFailed`, `ReconcileError` |
| `Available` | Every desired replica is ready and updated | Anything else | `AllReplicasReady`, `ReplicasUnavailable`, `ImagePullFailure`, `ProbeFailure`, `PolicyViolation`, `OverrideRequired`, `ValidationFailed`, `ReconcileError` |
| `Degraded` | A blocking policy violation or a technical fault | No blocking fault was detected | `PolicyViolation`, `OverrideRequired`, `ImagePullFailure`, `ProbeFailure`, `ReplicasUnavailable`, `RolloutTimeout`, `ValidationFailed`, `ReconcileError`, and `Reconciling` when `False` |
| `PolicyWarning` | At least one non-blocking policy deviation | No deviation | `PolicyDeviation` when `True`, `ValidationPassed` when `False` |

Every condition carries `observedGeneration` equal to the `BrunerApp`
generation being reconciled.

### Stage and message mapping

| Observed state | Stage | `Available` | `Degraded` | Reason | Message |
| --- | --- | --- | --- | --- | --- |
| `replicas: 0` requested | `Inspection` | False | False | `ReplicasUnavailable` | "Desired replica count is zero, so no pods are expected. Scale the application up to start production. The line is idle by request." |
| Pod unschedulable | `LineStopped` | False | True | `ReplicasUnavailable` | "Pod cannot be scheduled: `<scheduler message>`. Check node resources, taints and host port conflicts. No free bay on the factory floor." |
| `ErrImagePull` / `ImagePullBackOff` / `InvalidImageName` / `ImageInspectError` | `LineStopped` | False | True | `ImagePullFailure` | "Container image could not be pulled (`<reason>`). Verify image reference and registry access. Parts delivery failed." |
| `CrashLoopBackOff` | `LineStopped` | False | True | `ProbeFailure` | "Container is restarting repeatedly. Inspect container logs and the liveness probe configuration. The unit keeps falling off the line." |
| `ProgressDeadlineExceeded` | `LineStopped` | False | True | `RolloutTimeout` | "Deployment exceeded its progress deadline. Inspect pod events and container logs for the stalled replica. The conveyor stopped mid-shift." |
| Ready and updated replicas ≥ desired | `Showroom` | True | False | `AllReplicasReady` | "All `<n>` replicas are ready. Assembly completed and product cleared for the showroom." |
| Running but not ready | `QualityControl` | False | False | `ProbeFailure` | "Readiness probe has not succeeded for `<k>` of `<n>` replicas. Final inspection is not complete." |
| Replicas still being created | `Assembly` | False | False | `Reconciling` | "Deployment reports `<m>` of `<n>` replicas created. Assembly is in progress." |
| Blocking policy violation | `Inspection` | False | True | the violation reason | the violation message; `Reconciling=False` with "Reconciliation is paused until the policy problem is resolved. The line is held at inspection." |
| Reconcile or read error | `LineStopped` | False | True | `ReconcileError` | "Deployment could not be applied: `<error>`. Check controller permissions and the API server. The assembly order was rejected." (and the Service and observation variants) |

### Policy findings

| Check | Blocking when | Reason | Message |
| --- | --- | --- | --- |
| Image reference parse failure | always | `ValidationFailed` | "Container image reference is invalid: `<error>`. Correct spec.image before the rollout can continue. Parts order rejected at the gate." |
| Mutable tag | `latestTagPolicy: Reject` | `PolicyViolation` / `PolicyDeviation` | "Container image `<ref>` uses a mutable tag. Pin an immutable tag or a sha256 digest so the rollout is reproducible. Parts arrived without a batch number." |
| Registry not allowed | always, when `allowedRegistries` is non-empty | `PolicyViolation` | "Container registry `<host>` is not in the allowed registry list `<list>`. Use an approved registry. Supplier is not on the approved list." |
| Missing readiness probe | `qualityGate: strict` or `profile: strict` | `PolicyViolation` / `PolicyDeviation` | "Readiness probe is not defined in the specification. Set spec.readinessProbe so readiness is measured explicitly. Final inspection has no checklist." |
| Missing CPU and memory requests | `qualityGate: strict` or `profile: strict` | `PolicyViolation` / `PolicyDeviation` | "CPU and memory requests are not set in the specification. Set spec.resources.requests so the scheduler can plan capacity. Line capacity cannot be planned without part weights." |
| Image change under `profile: friday` without a matching override | always | `OverrideRequired` | "Image change from `<old>` to `<new>` is held back by the friday profile. Set spec.override.approved to true with a matching spec.override.image to continue. No unsigned changes go to the line before the weekend." |
| Non-loopback `hostIP` | never (normalised) | `PolicyDeviation` | "Host IP `<ip>` is not a loopback address and was replaced with 127.0.0.1. Brunernetes publishes applications on localhost only. The showroom has no street entrance." |

The `friday` guard only applies to a *change*: it compares
`status.appliedImage` with `spec.image` and does nothing on a first rollout,
when `status.appliedImage` is empty.

The readiness-probe and resource-request checks fire when the effective mode
requires them (`RequiresReadinessProbe` / `RequiresResourceRequests`) **or**
when `AvtoVAZ.spec.policy.requireReadinessProbe` /
`requireResourceRequests` is `true`; the quality gate then decides whether the
finding blocks or only warns.

### Events emitted

| Reason | Type | When |
| --- | --- | --- |
| `DeploymentApplied` | Normal | The Deployment was created or updated ("Deployment `<name>` was `<op>` with image `<image>`. Assembly instructions handed to the line.") |
| `ServiceApplied` | Normal | The Service was created or updated ("Service `<name>` was `<op>`. Delivery bay is open.") |
| the assessment reason | Normal, or Warning when degraded | Only when the verdict changed - the reason or the stage differs from what is already in status. A steady line stays quiet |
| `PolicyDeviation` | Warning | One event per warning finding |
| `PolicyViolation`, `OverrideRequired` | Warning | The blocking finding |
| `ValidationFailed` | Warning | An invalid operating mode |
| `ReconcileError` | Warning | The Deployment or the Service could not be applied |

### Error handling

| Failure | Handling |
| --- | --- |
| `BrunerApp` not found | Ignored, no requeue |
| `BrunerApp` being deleted | Return immediately; garbage collection handles the owned objects |
| `AvtoVAZ` list fails | Fall back to `factory.DefaultPolicy()`. Policy is degraded to defaults rather than the reconcile failing |
| Invalid effective mode | `Degraded=True` with `ValidationFailed`, stage `LineStopped`, requeue after 15 s |
| Blocking policy violation | `Degraded=True`, `Available=False`, `Reconciling=False`, stage `Inspection`, requeue after 15 s. Nothing is written to the cluster |
| Deployment or Service apply fails | Status written with `ReconcileError`, then the original error is returned so controller-runtime applies backoff |
| Pod or Deployment read fails | Status written with `ReconcileError`, error returned |
| Status update conflict (`409`) | `Requeue: true`, no error |
| Any other status update error | Returned to controller-runtime |

### Idempotency guarantees

- `BuildDeployment` and `BuildService` are pure functions of the `BrunerApp`
  and the effective mode.
- The Deployment selector is written only at creation time.
- `equalStatus` suppresses a status write when nothing but a timestamp would
  change, so a steady application produces zero writes per resync.
- Events are only emitted on a verdict transition, so the event stream does not
  grow while nothing happens.
- A non-loopback `hostIP` is normalised to `127.0.0.1` on every pass, so the
  generated pod spec converges rather than oscillating.

---

## Togliatti controller - specified, deferred to v0.2

Observes the single-node environment. It owns nothing and writes only status.

**Watched:** `Togliatti` (cluster-scoped); `v1 Node`; the `kube-system` pods for
CoreDNS and `kube-proxy`. **Owned:** nothing. **Triggers:** any change to the
`Togliatti` or the node, plus a periodic requeue every
`spec.probeIntervalSeconds` (default 60 s, range 10–3600).

Resolution: use `spec.nodeName` when set, otherwise the only node in the
cluster; a cluster with more than one node is reported as a violated assumption
rather than silently picking one.

Conditions:

| Condition | `True` when | Reason on success / failure |
| --- | --- | --- |
| `APIHealthy` | The API server answered the controller's read | `ProbeSucceeded` / `ProbeFailed` |
| `NodeReady` | The node's own `Ready` condition is `True` | `ProbeSucceeded` / `ProbeFailed` |
| `RuntimeAvailable` | `node.status.nodeInfo.containerRuntimeVersion` is present and matches `spec.expectedRuntime` | `ProbeSucceeded` / `ProbeFailed` |
| `CNIHealthy` | The node has no `NetworkUnavailable=True` condition and CoreDNS is ready | `ProbeSucceeded` / `ProbeFailed` |
| `LocalEndpointCapable` | The node can publish a loopback host port | `ProbeSucceeded` / `ProbeFailed` |
| `Available` | All of the above are `True` | `ProbeSucceeded` / `ProbeFailed` |

Status also reports `nodeName`, `kubernetesVersion`, `containerRuntime`,
`activeCapacityProfile`, `resources`, `schedulableForWorkloads`,
`localEndpointCapable` and `lastProbeTime`. The capacity profile is derived from
allocatable CPU and memory when `spec.capacityProfile` is empty: `compact` at
roughly 2 vCPU / 4 GB, `standard` at roughly 4 vCPU / 8 GB, `spacious` above.

A mismatch between `spec.expectedKubernetesVersion` and the observed kubelet
version is reported in the `Available` message and does not set `Degraded` -
the field's documentation states that a mismatch "is reported but does not stop
the line".

Events: `ProbeFailed` (Warning) on a condition transition to `False`,
`ProbeSucceeded` (Normal) on recovery. No event while conditions are stable.

Example messages: "Node is NotReady. Check kubelet and containerd. Factory floor
unavailable." and "Loopback host port publishing is unavailable because the
portmap CNI plugin is not configured. The showroom has no front door."

Error handling: a failed node read sets `APIHealthy=False` and requeues after
15 s. The controller never returns an error for an unhealthy environment -
an unhealthy environment is the thing it exists to report.

Idempotency: status is written only when a field or a condition actually
changes; `lastProbeTime` is updated on every successful probe, which is the one
intentional periodic write, and it is the reason this controller must not use
`equalStatus` alone to suppress writes.

---

## AvtoVAZ controller - specified, deferred to v0.2

Reports the effective factory policy and enforces the application limit.

**Watched:** `AvtoVAZ` (cluster-scoped); `BrunerApp` across all namespaces, so
that creating or deleting an application re-evaluates capacity.
**Owned:** nothing. **Triggers:** any change to an `AvtoVAZ` or to any
`BrunerApp`, plus a periodic resync on the shift cadence.

Status: `observedGeneration`; `effectiveMode` = `MergeModes(spec.defaultMode)`,
that is, the factory mode with every unset field filled from `DefaultMode()`;
`applicationCount` = the number of `BrunerApp` resources in the cluster;
`maxApplications` = the normalised limit.

Conditions:

| Condition | `True` when | `False` when | Reasons |
| --- | --- | --- | --- |
| `PolicyEnforced` | The policy was normalised and is being applied | The policy could not be read or normalised | `ValidationPassed` / `ValidationFailed` |
| `CapacityRespected` | `applicationCount ≤ maxApplications` | More applications exist than the limit allows | `ValidationPassed` / `CapacityExceeded` |

Capacity message (`factory.EvaluateCapacity`): "Cluster holds `<n>` BrunerApp
resources but the factory policy allows `<m>`. Delete the extra applications.
Only one product line fits on this floor."

Events: `CapacityExceeded` (Warning) on transition to over-limit,
`ValidationPassed` (Normal) on return to within-limit.

Error handling: a failed `BrunerApp` list leaves `applicationCount` untouched,
sets `PolicyEnforced=False` with `ReconcileError`, and requeues after 15 s.

Idempotency: the controller writes no objects at all; the only side effect is a
status update, suppressed when unchanged. Note that the `AvtoVAZ` controller
does **not** block application creation - that is the `BrunerApp` controller's
job - which is a direct consequence of evaluating policy in controllers rather
than in an admission webhook
([ADR 0002](adr/0002-crd-and-controller-boundary.md)).

---

## Konveer controller - specified, deferred to v0.2

Runs one delivery workflow for an already-built image. See
[ADR 0004](adr/0004-delivery-without-image-build.md).

**In v0.1 this workflow exists and runs, but in `brunerctl`, not in a
controller** (`internal/cli/konveer.go`, invoked by `brunerctl konveer apply
-f konveer.yaml`). The stage names, the phase values, the status fields and the
conditions below are exactly what the client-side implementation writes today;
the specification in this section is therefore both a description of current
behaviour and the contract the v0.2 controller must preserve. The differences
between the two, all of which disappear in v0.2, are:

| Aspect | v0.1, client-side | v0.2, controller |
| --- | --- | --- |
| Trigger | An explicit `brunerctl konveer apply` | Creation of the `Konveer`, by any client |
| Progress | Blocking, in-process, with step output on the terminal | Reconcile loop with a short requeue while non-terminal |
| Interruption | The delivery stops where it was; status is not advanced further | The controller resumes on the next reconcile |
| Rollout wait | `WaitForApp` polls the `BrunerApp` until the timeout (`spec.timeouts.rolloutSeconds`, default 300 s) | Requeue on `BrunerApp` status changes |
| Verify probe | An HTTP request from wherever `brunerctl` runs. If the endpoint is unreachable the stage is recorded as `Skipped`, not `Failed`, because `brunerctl` may be running off-node | A probe from inside the cluster |
| Events | None; output goes to the terminal | Events on the `Konveer` |

**Watched:** `Konveer` (namespaced); `BrunerApp` (to observe the delivery
target). **Owned:** nothing - the `BrunerApp` is created or updated but never
owned, so deleting a finished `Konveer` never deletes the application.
**Triggers:** creation of a `Konveer`; a status change on the referenced
`BrunerApp`; and a short requeue (2 s to 5 s) while the workflow is in a
non-terminal phase.

A `Konveer` whose `status.phase` is `Succeeded` or `Failed` and whose
`observedGeneration` equals `metadata.generation` is terminal and is not
reconciled again. Re-running a delivery means creating a new `Konveer`, not
editing an old one.

### Stages, phases and conditions

| Stage name | Phase | Sets on success | Sets on failure |
| --- | --- | --- | --- |
| `Validate` | `Validating` | `Validated=True`, reason `ValidationPassed` | `Validated=False`, `Degraded=True`, reason `ValidationFailed`, `PolicyViolation`, `OverrideRequired` or `CapacityExceeded`; phase `Failed` |
| `Apply` | `Applying` | stage `Succeeded`; `status.appliedImage` set | `Degraded=True`, reason `ReconcileError`; phase `Failed` |
| `Rollout` | `RollingOut` | `RolledOut=True`, reason `AllReplicasReady` | `RolledOut=False`, `Degraded=True`, reason `RolloutTimeout`; phase `Failed` |
| `Readiness` | `RollingOut` | stage `Succeeded` | reason `ProbeFailure` or `RolloutTimeout`; phase `Failed` |
| `Verify` | `Verifying` | `Verified=True`, reason `EndpointVerified`; `status.verifiedEndpoint` set | `Verified=False`, reason `EndpointUnverified`; phase `Failed`. **Skipped**, not failed, when the controller cannot reach the node's loopback endpoint or `spec.verify.enabled` is `false` |
| - | `Succeeded` | `Succeeded=True`, reason `ValidationPassed`; `status.completionTime` set | `Succeeded=False` with the terminal reason |

`status.stages` records every stage in execution order with its `state`
(`Pending`, `Running`, `Succeeded`, `Failed`, `Skipped`), a message, a
`startTime` and a `completionTime`.

### Stage detail

**Validate.** Governed by `spec.checks`, all defaulting to `true`.
`imageReference` runs `ParseImageReference`; `policy` evaluates the same
`factory.EvaluateApp` rules against the prospective `BrunerApp`, so a `Konveer`
and a direct `BrunerApp` edit produce identical verdicts; `capacity` runs
`factory.EvaluateCapacity` against the current application count. No registry is
contacted at any point.

**Apply.** Create the `BrunerApp` named by `spec.appRef` in the same namespace
if it does not exist, otherwise update it. Only `spec.image` and the fields
present in `spec.appTemplate` (`replicas`, `port`, `healthPath`, `mode`,
`localExposure`) are written; every other field of an existing `BrunerApp` is
left alone. `spec.override` is copied onto the `BrunerApp` so the `friday`
guard sees the approval.

**Rollout and readiness.** Poll the `BrunerApp` status. The rollout stage ends
when `status.observedGeneration` has caught up and `status.appliedImage` equals
the delivered image; the readiness stage ends when `Available=True`. The two
waits are bounded by `spec.timeouts.rolloutSeconds` (default 300) and
`spec.timeouts.readinessSeconds` (default 180). A `Degraded=True` on the
`BrunerApp` fails the delivery immediately rather than waiting for the timeout -
there is no point waiting out 300 seconds for an `ImagePullBackOff`.

**Verify.** HTTP `GET` against `BrunerApp.status.localEndpoint` +
`spec.verify.path`, expecting `spec.verify.expectedStatus`. The controller pod
is not in the host network namespace, so the node's loopback endpoint is
frequently unreachable from it; that case is `Skipped` with an explicit message,
never `Failed`. A connection that is established but answers the wrong status is
a genuine failure.

### Events emitted

| Reason | Type | When |
| --- | --- | --- |
| `ValidationPassed` | Normal | Validation succeeded |
| `ValidationFailed`, `PolicyViolation`, `OverrideRequired`, `CapacityExceeded` | Warning | Validation failed |
| `AllReplicasReady` | Normal | The rollout and readiness stages completed |
| `RolloutTimeout` | Warning | A wait exceeded its timeout |
| `EndpointVerified` | Normal | The endpoint answered the expected status |
| `EndpointUnverified` | Warning | The endpoint answered the wrong status or refused the connection |
| `ReconcileError` | Warning | The `BrunerApp` could not be written |

Example messages: "Container image reference is invalid: image reference has an
empty tag. Correct spec.image before the rollout can continue. Parts order
rejected at the gate." and "All replicas are ready and the loopback endpoint
answered 200. Assembly completed and product cleared for the showroom."

### Error handling and idempotency

| Failure | Handling |
| --- | --- |
| `Konveer` not found | Ignored |
| Referenced `BrunerApp` write fails | `Degraded=True`, `ReconcileError`, phase `Failed`. The stage records the API error verbatim |
| Rollout or readiness timeout | Phase `Failed` with `RolloutTimeout`. The `BrunerApp` is **not** rolled back - the delivery reports what happened; reverting is an explicit user action |
| Endpoint unreachable from the controller | `Verify` stage `Skipped`, delivery still `Succeeded` |
| Endpoint answers the wrong status | `Verified=False`, `EndpointUnverified`, phase `Failed` |
| Status update conflict | `Requeue: true` |

Idempotency: the apply stage is a converging write of a fixed field set, so
re-running it changes nothing once applied. Stage start times are written once
and never overwritten, so a requeue mid-workflow does not restart the clock. A
terminal `Konveer` is skipped entirely, so a controller restart does not re-run
a finished delivery.

## Related documents

- [api.md](api.md) - field-level reference
- [architecture.md](architecture.md) - component and sequence diagrams
- [ADR 0002: CRD and controller boundary](adr/0002-crd-and-controller-boundary.md)
- [ADR 0003: Localhost-only exposure](adr/0003-localhost-only-exposure.md)
- [ADR 0004: Delivery without image build](adr/0004-delivery-without-image-build.md)
- [testing.md](testing.md) - how these behaviours are verified
- [roadmap.md](roadmap.md) - when the deferred controllers land
