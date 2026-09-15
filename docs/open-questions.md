# Open questions

Decisions that are not made yet and that block or shape further implementation.
Each entry states the question, why it matters now, the options with their real
costs, and a recommendation. A recommendation here is a starting position for a
review, not a decision - once one is taken, it moves out of this file and into
an [ADR](adr/).

Questions are ordered by how soon they need an answer.

---

## Q1. Does the Helm chart delete the CRDs on uninstall?

**Status:** blocks the completion of the chart, and therefore v0.1.

Helm's `crds/` directory installs CRDs but never upgrades or deletes them.
Templating them under `templates/` gives full lifecycle management, including
deletion - and deleting a CRD deletes every custom resource of that kind, with
no confirmation and no recovery. `brunerctl uninstall` runs `helm uninstall`
first and namespace deletion second, so the order of destruction differs between
the two placements.

| Option | Consequence |
| --- | --- |
| A. `crds/` directory | CRDs survive `helm uninstall`; `BrunerApp` resources survive with them. Upgrading a CRD requires a manual `kubectl apply`. Leaves residue after an uninstall that claims to be complete. |
| B. `templates/` with `helm.sh/resource-policy: keep` | Full upgrade support, explicit retention on delete. Same residue, but deliberate and annotated. |
| C. `templates/` without the annotation | `helm uninstall` removes the CRDs and cascades to every CR. Clean, and a footgun for anyone who runs `helm uninstall` expecting to reinstall. |

**Recommendation: B.** Brunernetes needs CRD upgrades - the API is `v1alpha1`
and will change - and the destruction of user resources should be an explicit
act. `brunerctl uninstall` can then delete the CRDs itself as a named step,
where the intent is unambiguous and the command is the one the user typed.

---

## Q2. How is the host port allocated when more than one application is allowed?

**Status:** blocks the `maxApplications > 1` work in v0.2.

Today `hostPort` defaults to `30080` and `maxApplications` is `1`, so the
question does not arise. The moment a second application is permitted, two pods
compete for one host port. The loser does not fail cleanly: it stays `Pending`
with a scheduling message that does not obviously name the cause.

| Option | Consequence |
| --- | --- |
| A. Explicit per-application `hostPort` | Simple, honest, no hidden state. The user must track allocations and will eventually collide. Collisions are at least self-inflicted and visible. |
| B. Controller-allocated from a range, recorded in `AvtoVAZ.status` | No collisions. Introduces allocation state that must survive controller restarts and be reclaimed on deletion - a small distributed allocator, which is more machinery than the problem deserves. |
| C. One shared reverse proxy on `30080`, routing by path or host header | One host port for any number of applications, and closest to how real ingress works. It is an ingress in all but name, which [ADR 0003](adr/0003-localhost-only-exposure.md) deliberately avoided. |
| D. Keep `maxApplications: 1` permanently | No question to answer. Constrains the platform to demonstrating exactly one thing. |

**Recommendation: A for v0.2, with a pre-admission collision check.** Validate
the requested `hostPort` against other `BrunerApp` resources in the reconciler
and surface a clear `PolicyWarning` before the pod goes `Pending`. Revisit C
only if a concrete use case for many simultaneous applications appears;
inventing a router before then is speculative.

---

## Q3. When does the Konveer workflow move from `brunerctl` into a controller?

**Status:** the largest behavioural change planned for v0.2.

`internal/cli/konveer.go` currently runs `Validate -> Apply -> Rollout ->
Verify` in-process and writes `Konveer.status` itself. It works, and it has one
structural defect: a delivery is bound to a terminal session. Interrupt the
command and the resource is frozen mid-phase with nothing to resume it. A
`Konveer` created by `kubectl apply` alone does nothing at all, which makes the
CRD misleading to anyone who has used Kubernetes before.

| Option | Consequence |
| --- | --- |
| A. Move to a controller in v0.2, delete the client path | The resource behaves as a Kubernetes resource should. Requires a controller-side rollout wait and an in-cluster endpoint probe, and `brunerctl konveer apply` becomes apply-then-watch. |
| B. Keep both, CLI as the default | Duplicate implementations of the same contract drift. Two code paths, one specification, no way to keep them equal without testing both. |
| C. Keep the client-side path indefinitely | Avoids the work and permanently misrepresents what a `Konveer` is. |

**Recommendation: A, with a transition gate.** Before deleting the client path,
require a test that runs both implementations over a shared fixture set and
asserts identical `status`. That gate is what makes the migration a refactor
rather than a rewrite.

The in-cluster verify probe needs its own decision: the controller runs in a pod
and cannot reach the node's `127.0.0.1:30080`. It must probe the ClusterIP
Service instead, which verifies the application but not the loopback binding -
the exact thing a user most wants confirmed. Probing the host binding from
inside the cluster would require host networking on the controller, which is a
privilege increase that risk 5 in [security.md](security.md) argues against.
The likely answer is that the controller verifies the Service and `brunerctl`
additionally verifies the loopback endpoint when it happens to run on the node,
reporting the two separately.

---

## Q4. Does `Togliatti` describe the environment or assert it?

**Status:** blocks the `Togliatti` reconciler design in v0.2.

`Togliatti.spec` carries `expectedKubernetesVersion` and `expectedRuntime`. The
word "expected" implies a comparison, but nothing today defines what happens
when reality differs: a node running `v1.32.4` against an expectation of
`v1.32.3` may be fine, may be a drift signal, or may be a reason to stop.

| Option | Consequence |
| --- | --- |
| A. Purely descriptive; status records what is observed | Simplest. The `expected*` fields become documentation with no behaviour, which is a poor name for a field that does nothing. |
| B. Assertive; a mismatch sets `EnvironmentReady=False` | Meaningful, and brittle: a routine patch upgrade turns the factory red until someone edits the CR. |
| C. Advisory; a mismatch sets a drift condition but never blocks | Visible, non-blocking, consistent with the advisory-policy stance of [ADR 0002](adr/0002-crd-and-controller-boundary.md). |
| D. Semver-aware; exact minor, tolerant patch | Matches how Kubernetes versions actually move. Requires a comparison rule that must itself be specified and tested. |

**Recommendation: C combined with D.** Compare on major and minor, tolerate
patch drift, and surface any difference as a non-blocking condition with both
values in the message. `EnvironmentReady` should reflect whether the environment
*works*, not whether it matches a string someone typed.

---

## Q5. Should the workload pod security context set `runAsNonRoot`?

**Status:** a security decision with a compatibility cost; scheduled for v0.3 but
worth deciding earlier.

The built security context disables privilege escalation, drops all
capabilities and applies the `RuntimeDefault` seccomp profile, but omits
`runAsNonRoot` and `runAsUser`. On a node that is also the control plane
(risk 1 and risk 6 in [security.md](security.md)), container root is a shorter
path to the host than it should be.

| Option | Consequence |
| --- | --- |
| A. Leave as is | Any image works, including the many that default to UID 0. Weakest posture. |
| B. `runAsNonRoot: true` always | Strongest default. Breaks every image that needs UID 0, including the stock nginx image, with a `CreateContainerConfigError` that is not self-explanatory. |
| C. Tie it to the profile: required under `strict`, off otherwise | Consistent with how `requireReadinessProbe` and `requireResourceRequests` already work. The security posture then depends on a field named for quality, not security. |
| D. An explicit `BrunerApp` field with a policy check in `AvtoVAZ` | Most precise, and adds API surface for something most users will never set. |

**Recommendation: C, with a clear failure message.** The profile mechanism
already exists and users already understand that `strict` means stricter. The
decisive detail is the message: if a pod cannot start because the image wants
root, `BrunerApp` status must say exactly that, not leave a raw
`CreateContainerConfigError` for the user to decode.

---

## Q6. How are images made available to the node without a registry?

**Status:** affects the demo experience today; unresolved rather than blocking.

Brunernetes never builds images ([ADR 0004](adr/0004-delivery-without-image-build.md)),
and `BrunerApp` has no `imagePullSecrets` field. That leaves public registries
and pre-pulled images, and a user with a locally built demo image has no
documented path onto the node.

| Option | Consequence |
| --- | --- |
| A. Document `ctr -n k8s.io images import` and `imagePullPolicy: Never` | Zero code. Requires root and knowledge of containerd's namespaces. Currently the only answer. |
| B. `brunerctl image load <tar>` | One small command that makes the demo path smooth. Places `brunerctl` in the business of talking to the container runtime, which risk 7 in [security.md](security.md) argues against. |
| C. Add `imagePullSecrets` to `BrunerApp` | Enables private registries. Introduces secret handling into an API that currently has none, and that absence is itself a mitigation (risk 15). |
| D. Ship the demo image on a public registry | Simplest for the user. Requires publishing and maintaining a public artefact, and a release process that does not yet exist. |

**Recommendation: A now, D at the first tagged release, B only if the demo turns
out to be the common case.** C should wait until someone has an actual private
registry requirement; adding secret handling speculatively expands both the API
and the threat model.

---

## Q7. Should the uninstall iptables flush stay unconditional?

**Status:** small, contained, and currently a destructive default.

Uninstall step 4 runs `iptables -F`, `-t nat -F`, `-t mangle -F` and `-X`. That
removes every rule in those tables, not only Kubernetes rules. On a developer
laptop it is harmless. On a host with its own firewall it silently disables it
until the firewall service restarts.

| Option | Consequence |
| --- | --- |
| A. Keep it unconditional | Reliable cleanup of `kube-proxy` and portmap leftovers. Collateral damage on any host with its own rules. |
| B. `--flush-iptables`, off by default, recommend a reboot instead | Safe by default. Some users will skip the reboot and be confused by stale rules. |
| C. Delete only the Kubernetes chains by name | Precise, and coupled to `kube-proxy` and portmap internals that change between versions. |

**Recommendation: B.** `kubeadm reset` already removes most of what matters, and
the closing message already tells the user to reboot. An uninstall should not be
capable of disabling a host's firewall as a side effect.

---

## Q8. What is the supported upgrade path for the pinned Kubernetes version?

**Status:** needed before anyone runs a Brunernetes host for more than a few
weeks; scheduled for v0.3.

Packages are held with `apt-mark hold` at `1.32.3-1.1`, which is what makes the
pin stick. Nothing defines how a host moves to the next patch version.

| Option | Consequence |
| --- | --- |
| A. Uninstall and reinstall | Zero new code, already tested by the uninstall flow. Destroys the cluster and every workload. Defensible for a development platform. |
| B. `brunerctl upgrade` wrapping `kubeadm upgrade` | Proper in-place upgrade. Meaningful work, and it needs its own preflight, failure handling and rollback story. |
| C. Document the manual `kubeadm upgrade` procedure | No code, honest, puts the burden on the user. |

**Recommendation: A for v0.2, C for v0.3, B only if demand appears.** A
single-node development cluster with one stateless application has very little
worth preserving across an upgrade, and the reinstall path is exercised by every
test run.

---

## Q9. When does the API graduate from `v1alpha1`?

**Status:** not urgent, but the answer determines how freely fields can change
in v0.2.

`bruner.dev/v1alpha1` signals that anything may change without a conversion
path. That freedom is genuinely useful while three reconcilers are still
unwritten.

| Option | Consequence |
| --- | --- |
| A. Stay `v1alpha1` through v0.2, promote in v0.3 | Full freedom exactly where it is needed. Delays a stability signal. |
| B. Promote to `v1beta1` at v0.2 | Signals maturity ahead of the implementation and forces conversion webhooks for changes that the missing reconcilers will probably require. |
| C. Stay `v1alpha1` indefinitely | Honest for a demonstration platform, and a signal that the project is not serious about its own API. |

**Recommendation: A.** Promote once the field set has survived a full release
cycle without change - that is evidence of stability, whereas a version bump on
its own is only an assertion of it.

---

## Q10. Does the "Build the Konveer" mini-game belong in this repository?

**Status:** affects v0.2 scope and the repository's identity.

The game is a teaching device: assemble a delivery conveyor under the same
constraints the real system imposes, so that a player understands why
`maxSurge: 0` behaves the way it does. It is not a platform feature.

| Option | Consequence |
| --- | --- |
| A. In `demo/`, served by the demo workload | Zero extra infrastructure, ships with the thing it teaches. Adds frontend code to an infrastructure repository and invites scope creep. |
| B. A separate repository, linked from the docs | Clean separation. Another repository to maintain, and it will drift from the platform it explains. |
| C. Not built | Nothing to maintain. Loses the one artefact that makes rollout semantics intuitive. |

**Recommendation: A, with a hard boundary.** Keep it in `demo/`, static, with no
backend, no network call after page load and no dependency on the controller,
and require that deleting its directory breaks nothing. If it ever needs a
build step or a server, that is the signal to move it to B.

---

## How to close a question

1. Take the decision in a review with the trade-offs on the table.
2. If it changes an architectural boundary, write an ADR in [adr/](adr/) and
   link it from the relevant documents.
3. If it is an implementation detail, record it in the document it affects.
4. Remove the entry from this file. This file should shrink as the project
   matures; a growing open-questions list is a signal in itself.

## Related documents

- [roadmap.md](roadmap.md) - the phases these questions gate
- [security.md](security.md) - the risk register behind Q5 and Q7
- [api.md](api.md) - the field set behind Q4 and Q9
- [reconciliation.md](reconciliation.md) - the controller status behind Q3
- [installation.md](installation.md) - the install and uninstall flows behind Q1, Q7 and Q8
