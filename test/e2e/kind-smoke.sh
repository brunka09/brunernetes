#!/usr/bin/env bash
#
# End to end smoke test for the Brunernetes controller.
#
# The installer itself needs a real host: it changes kernel modules, installs
# containerd and runs kubeadm, so it cannot run inside a CI container. What can
# be verified in CI is everything above that line: the CRDs install, the chart
# renders and deploys, the controller becomes ready, a BrunerApp produces a
# Deployment and a Service, the policy path blocks what it should, and deleting
# the application removes its objects.
#
# Requirements: docker, kind, kubectl, helm.
#
# Usage: ./test/e2e/kind-smoke.sh

set -euo pipefail

KIND_CLUSTER="${KIND_CLUSTER:-brunernetes-e2e}"
IMAGE="${IMAGE:-ghcr.io/brunka09/brunernetes:0.1.0}"
CHART="${CHART:-charts/brunernetes}"
NAMESPACE="${NAMESPACE:-brunernetes-system}"
DEMO_NAMESPACE="${DEMO_NAMESPACE:-brunernetes-e2e-demo}"
KEEP_CLUSTER="${KEEP_CLUSTER:-false}"

step() { printf '\n=== %s\n' "$1"; }
fail() { printf '\nFAILED: %s\n' "$1" >&2; exit 1; }

cleanup() {
  local status=$?
  if [ "$status" -ne 0 ]; then
    echo
    echo "The smoke test failed. Controller logs follow."
    kubectl -n "$NAMESPACE" logs deployment/brunernetes --tail=100 2>/dev/null || true
    kubectl get events -A --sort-by=.lastTimestamp 2>/dev/null | tail -30 || true
  fi
  if [ "$KEEP_CLUSTER" != "true" ]; then
    step "Deleting the kind cluster"
    kind delete cluster --name "$KIND_CLUSTER" >/dev/null 2>&1 || true
  fi
  exit "$status"
}
trap cleanup EXIT

for tool in docker kind kubectl helm go; do
  command -v "$tool" >/dev/null 2>&1 || fail "$tool is required but was not found on PATH."
done

step "Building the controller image"
docker build -t "$IMAGE" .

step "Creating the kind cluster $KIND_CLUSTER"
kind delete cluster --name "$KIND_CLUSTER" >/dev/null 2>&1 || true
kind create cluster --name "$KIND_CLUSTER" --wait 180s
kubectl cluster-info

step "Loading the controller image into the cluster"
kind load docker-image "$IMAGE" --name "$KIND_CLUSTER"

step "Installing the chart"
helm upgrade --install brunernetes "$CHART" \
  --namespace "$NAMESPACE" --create-namespace \
  --set image.repository="${IMAGE%:*}" \
  --set image.tag="${IMAGE##*:}" \
  --set image.pullPolicy=Never \
  --wait --timeout 5m

step "Waiting for the controller to become ready"
kubectl -n "$NAMESPACE" rollout status deployment/brunernetes --timeout=180s

step "Checking that every custom resource definition is established"
for crd in brunerapps avtovazs togliattis konveers; do
  kubectl wait --for=condition=Established "crd/${crd}.bruner.dev" --timeout=60s
done

step "Creating the factory resources"
kubectl apply -f config/samples/bruner.dev_v1alpha1_togliatti.yaml
kubectl apply -f config/samples/bruner.dev_v1alpha1_avtovaz.yaml

step "Deploying an application"
kubectl create namespace "$DEMO_NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -
cat <<EOF | kubectl apply -f -
apiVersion: bruner.dev/v1alpha1
kind: BrunerApp
metadata:
  name: smoke
  namespace: $DEMO_NAMESPACE
spec:
  image: ghcr.io/nginxinc/nginx-unprivileged:1.27-alpine
  port: 8080
  replicas: 1
  healthPath: /
EOF

step "Waiting for the controller to build the workload"
kubectl -n "$DEMO_NAMESPACE" wait --for=jsonpath='{.status.deploymentName}'=smoke \
  brunerapp/smoke --timeout=120s
kubectl -n "$DEMO_NAMESPACE" get deployment smoke -o name >/dev/null \
  || fail "The controller did not create the Deployment."
kubectl -n "$DEMO_NAMESPACE" get service smoke -o name >/dev/null \
  || fail "The controller did not create the Service."

service_type=$(kubectl -n "$DEMO_NAMESPACE" get service smoke -o jsonpath='{.spec.type}')
[ "$service_type" = "ClusterIP" ] \
  || fail "The Service must be a ClusterIP Service, but it is $service_type."

owner=$(kubectl -n "$DEMO_NAMESPACE" get deployment smoke -o jsonpath='{.metadata.ownerReferences[0].kind}')
[ "$owner" = "BrunerApp" ] \
  || fail "The Deployment must be owned by the BrunerApp, but the owner is $owner."

step "Checking the reported status"
kubectl -n "$DEMO_NAMESPACE" get brunerapp smoke -o yaml
endpoint=$(kubectl -n "$DEMO_NAMESPACE" get brunerapp smoke -o jsonpath='{.status.localEndpoint}')
case "$endpoint" in
  http://127.0.0.1:*) ;;
  *) fail "The reported endpoint must be on the loopback address, but it is '$endpoint'." ;;
esac

step "Letting the first rollout settle"
# The pod carries a host port, so a replacement pod cannot be scheduled while
# the previous one still holds it. Waiting here keeps the next step honest.
kubectl -n "$DEMO_NAMESPACE" rollout status deployment/smoke --timeout=180s

step "Checking that an image change reaches the Deployment"
kubectl -n "$DEMO_NAMESPACE" patch brunerapp smoke --type=merge \
  -p '{"spec":{"image":"ghcr.io/nginxinc/nginx-unprivileged:1.27"}}'
current=""
for _ in $(seq 1 60); do
  current=$(kubectl -n "$DEMO_NAMESPACE" get deployment smoke -o jsonpath='{.spec.template.spec.containers[0].image}')
  [ "$current" = "ghcr.io/nginxinc/nginx-unprivileged:1.27" ] && break
  sleep 2
done
[ "$current" = "ghcr.io/nginxinc/nginx-unprivileged:1.27" ] \
  || fail "The image change never reached the Deployment."

step "Checking that the API server refuses a non loopback host address"
# The CRD pattern is the only validation layer in this design, so this is the
# check that keeps the distribution loopback only.
if cat <<EOF | kubectl apply -f - 2>/dev/null
apiVersion: bruner.dev/v1alpha1
kind: BrunerApp
metadata:
  name: public-attempt
  namespace: $DEMO_NAMESPACE
spec:
  image: ghcr.io/nginxinc/nginx-unprivileged:1.27-alpine
  port: 8080
  healthPath: /
  localExposure:
    enabled: true
    hostIP: 0.0.0.0
    hostPort: 30081
EOF
then
  kubectl -n "$DEMO_NAMESPACE" delete brunerapp public-attempt --ignore-not-found
  fail "The API server accepted a host address outside 127.0.0.0/8."
fi
echo "The API server rejected the manifest, which is the expected result."

step "Checking that the Service stays a ClusterIP Service"
service_count=$(kubectl -n "$DEMO_NAMESPACE" get services -o jsonpath='{range .items[*]}{.spec.type}{"\n"}{end}' | sort -u)
[ "$service_count" = "ClusterIP" ] \
  || fail "Only ClusterIP Services may exist, but these types were found: $service_count"

step "Deleting the application"
kubectl -n "$DEMO_NAMESPACE" delete brunerapp smoke --wait=true
for _ in $(seq 1 30); do
  kubectl -n "$DEMO_NAMESPACE" get deployment smoke >/dev/null 2>&1 || break
  sleep 2
done
kubectl -n "$DEMO_NAMESPACE" get deployment smoke >/dev/null 2>&1 \
  && fail "Deleting the application did not remove its Deployment."

step "Uninstalling the chart"
helm uninstall brunernetes --namespace "$NAMESPACE" --wait

echo
echo "The smoke test passed. Every stage of the line reported a good part."
