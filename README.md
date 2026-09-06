# Brunernetes

Brunernetes is a lightweight single-node Kubernetes developer experience layer themed around Togliatti, AvtoVAZ, factory production, and an assembly line.

This initial repository revision defines the `bruner.dev/v1alpha1` Kubernetes API only. It includes Custom Resource types, structural CRDs, OpenAPI schema validation, status subresources, generated DeepCopy implementations, sample manifests, and unit tests for validation-sensitive helpers.

## Included APIs

| Kind | Scope | Purpose |
|---|---|---|
| `Togliatti` | Cluster | Represents the single-node factory environment and its health summary. |
| `AvtoVAZ` | Cluster | Represents factory-wide defaults, safety, quality, and capacity policy. |
| `BrunerApp` | Namespaced | Represents the one locally exposed application managed by Brunernetes. |
| `Konveer` | Namespaced | Represents a lightweight delivery workflow for an existing image. |

## Deliberately excluded

This revision does not contain controllers, a controller manager, a CLI, an admission webhook, Helm charts, an installer, a demo application, or CI workflows.

## API generation

Install `controller-gen` and run:

```bash
make generate
make manifests
make test
```

The repository commits generated artifacts so CRD schemas and DeepCopy implementations are inspectable without local code generation.

## API examples

Examples are available in [`config/samples`](config/samples). Apply the cluster-scoped resources first, then the namespaced examples:

```bash
kubectl apply -f config/samples/bruner_v1alpha1_togliatti.yaml
kubectl apply -f config/samples/bruner_v1alpha1_avtovaz.yaml
kubectl apply -f config/samples/bruner_v1alpha1_brunerapp.yaml
kubectl apply -f config/samples/bruner_v1alpha1_konveer.yaml
```

The manifests define desired state only. They do not create workloads until controllers are implemented in a later revision.
