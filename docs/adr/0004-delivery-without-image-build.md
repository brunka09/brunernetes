# ADR 0004: Konveer delivers existing images only - no builder, no registry

- Status: Accepted
- Date: 2026-09-07
- Deciders: Brunernetes maintainers
- Applies to: v0.1 (MVP)

## Context

`Konveer` is the delivery workflow resource: it takes an image, puts it on the
application, and reports whether the result actually serves traffic. The name
and the theme (a conveyor line) invite an obvious extension - building the image
too, so that a user could point `Konveer` at a Git repository and get a running
application. That would turn Brunernetes into a miniature CI/CD system.

The specification closes this off in two places: `Konveer` "MUST NOT build
images in MVP", and the non-goals list "no complete CI system, no image builder
or registry". The API reflects it: `KonveerSpec` has `image` (an
"already-built image reference to deliver") and no source, no build context, no
Dockerfile path, no build arguments.

The question this ADR settles is not whether to build in v0.1 - that is given -
but why the boundary is drawn there and what it costs.

## Decision

`Konveer` operates exclusively on **image references that already exist in a
registry reachable by the node's containerd**. Its stages are:

1. **Validate.** Controlled by `spec.checks`
   (`imageReference`, `policy`, `capacity`, all defaulting to `true`).
   `ParseImageReference` in `api/v1alpha1/validation.go` parses the reference
   without contacting any registry: it splits an optional `sha256:` digest,
   infers the registry host from the first path segment when it contains `.` or
   `:` or equals `localhost`, defaults the registry to `docker.io`, expands bare
   Docker Hub names to `library/<name>`, and defaults a missing tag to
   `latest`. `IsMutable` then reports whether the reference is tag-based and
   therefore subject to `AvtoVAZ.spec.policy.latestTagPolicy`, and
   `RegistryAllowed` checks it against
   `AvtoVAZ.spec.policy.allowedRegistries`.
2. **Apply.** Write `spec.image` and the optional `spec.appTemplate` fields onto
   the `BrunerApp` named by `spec.appRef` in the same namespace, creating the
   `BrunerApp` when it does not exist.
3. **Roll out.** Wait for the Deployment owned by the `BrunerApp`, bounded by
   `spec.timeouts.rolloutSeconds` (default 300).
4. **Wait for readiness.** Bounded by `spec.timeouts.readinessSeconds`
   (default 180).
5. **Verify.** Optional HTTP check against the loopback endpoint
   (`spec.verify.path`, default `/healthz`; `spec.verify.expectedStatus`,
   default 200), skipped rather than failed when the controller cannot reach the
   node's loopback interface.

Brunernetes ships **no** builder (no BuildKit, Kaniko, Buildah, `docker build`),
**no** registry (no distribution, no Zot, no `containerd` image import service),
and **no** credential brokering beyond `BrunerApp.spec.imagePullSecrets`, which
is a plain list of `LocalObjectReference` handed to the pod spec.

The image is expected to come from one of:

- a public registry (`ghcr.io`, `docker.io`, ...),
- a private registry the user already has, with credentials in a
  `Secret` referenced by `spec.imagePullSecrets`,
- an image the user side-loaded onto the node with
  `ctr -n k8s.io images import`, combined with
  `spec.imagePullPolicy: IfNotPresent` or `Never`.

The demo application is built and published out of band; see
[installation.md](../installation.md).

## Alternatives considered

### Bundle a builder (BuildKit or Kaniko) as an in-cluster Job

Rejected:

- A builder is a large, stateful, resource-hungry component. On a 2 vCPU / 4 GB
  host it competes directly with the control plane, which shares the node
  ([ADR 0001](0001-single-node-upstream-kubernetes.md)) - the most likely
  outcome of a build is an unresponsive API server.
- A builder needs a cache volume, which needs a storage decision the project
  has deliberately not made.
- Build inputs need a source story: Git URL, ref, credentials, submodules,
  shallow clones, private repositories. That is an entire subsystem with its own
  API surface, secrets handling and failure modes.
- Builds are the least deterministic part of any pipeline. Folding them into the
  same resource as "did the endpoint answer 200" makes every failure ambiguous:
  the user cannot tell whether the line stopped because of their Dockerfile,
  the network, the registry, the rollout or the probe.

### Bundle a local registry (`registry:2` / Zot) so builds have somewhere to push

Rejected as a consequence of rejecting the builder: with no builder there is
nothing to push. A registry alone would only be a caching proxy, which is a
performance optimisation, not an MVP requirement, and it would add TLS,
authentication and disk-growth concerns.

### Shell out to the host's `docker`/`nerdctl` from the controller

Rejected outright. It would require mounting the containerd socket into the
controller pod, which converts the controller's compromise into a full node
compromise (see [security.md](../security.md)), and it would make the
controller depend on host tooling that may not exist.

### Delegate to an external CI system and have `Konveer` only observe

This is effectively the chosen design, minus the pretence of orchestrating the
build. Adopted.

## Consequences

**Positive**

- `Konveer` has one job - deliver and verify - and its status is unambiguous.
  Every stage failure points at exactly one subsystem.
- The controller needs no privileged access: no containerd socket, no host
  mounts, no build cache volume. Its RBAC stays within the Brunernetes CRDs plus
  Deployments, Services, Pods, Events and Nodes.
- Validation is fully offline and therefore fast, deterministic and unit
  testable. `ParseImageReference` contacts nothing, so the same input always
  produces the same verdict - see [testing.md](../testing.md).
- The resource footprint of a delivery is a few HTTP calls to the API server,
  which is what a 2 vCPU host can afford.
- The security boundary is clean: Brunernetes never handles source code and
  never produces artefacts, so it cannot be the origin of a supply-chain
  artefact. It only consumes references.

**Negative**

- The inner loop is not closed. A developer changing the demo application must
  build and push the image with their own tooling before `Konveer` can do
  anything. For the "Factory Showroom" static site this is a small step, but it
  is a step.
- Because Brunernetes never resolves a reference against a registry, an
  unreachable or non-existent image is only discovered when the kubelet tries to
  pull it. The validation stage passes and the failure appears later as an
  `ImagePullBackOff`, surfaced as `Degraded=True` with reason
  `ImagePullFailure` and the message "Container image could not be pulled.
  Verify image reference and registry access. Parts delivery failed."
- Tag mutability cannot be resolved to a digest. Brunernetes can tell the user
  that `:latest` is mutable (`IsMutable`) and apply `latestTagPolicy`, but it
  cannot record which digest was actually deployed, because it does not query
  the registry. `status.appliedImage` therefore holds the *reference*, not a
  digest. This is the main correctness gap of the decision and is tracked in
  [open-questions.md](../open-questions.md).
- Air-gapped use requires manual `ctr images import` plus
  `imagePullPolicy: Never`; there is no supported side-load path in the API.

## Trade-offs

| Dimension | Deliver only (chosen) | Deliver + build |
| --- | --- | --- |
| Resource cost on a 2 vCPU / 4 GB node | Negligible | Dominant |
| Controller privilege required | Standard RBAC | Containerd socket or a privileged build pod |
| Failure diagnosability | One subsystem per stage | Build failures blur into delivery failures |
| API surface | `image` + template + checks + timeouts + verify | Plus source, ref, credentials, context, cache, push target |
| Inner developer loop | Open (build externally) | Closed |
| Fits the stated non-goals | Yes | No |
| Digest pinning possible | Only if the user supplies a digest | Yes, the builder knows it |

## Related decisions

- [ADR 0001: Single node on upstream Kubernetes](0001-single-node-upstream-kubernetes.md)
- [ADR 0002: CRD and controller boundary](0002-crd-and-controller-boundary.md)
- [api.md](../api.md), [reconciliation.md](../reconciliation.md),
  [security.md](../security.md)
