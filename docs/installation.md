# Installation and uninstallation

This document describes the exact ordered flow `brunerctl install` performs, the
inverse flow `brunerctl uninstall` performs, every command that needs `sudo` and
why, and how dry-run mode behaves. The authoritative implementation is
`internal/cli/install.go`, `internal/cli/uninstall.go`,
`internal/cli/preflight.go` and `internal/cli/versions.go`.

Design rationale for the substrate is in
[ADR 0001](adr/0001-single-node-upstream-kubernetes.md); for the loopback
endpoint in [ADR 0003](adr/0003-localhost-only-exposure.md).

## Supported hosts

| Requirement | Value | Enforcement |
| --- | --- | --- |
| Architecture | x86_64 (`amd64`) | Blocking preflight failure otherwise |
| Distribution | Ubuntu or Debian | Warning on anything else; package installation is then skipped and must be done manually |
| CPU | 2 cores minimum | Blocking |
| Memory | 4 GiB minimum (5 % tolerance) | Blocking |
| Free disk on `/` | 20 GiB recommended | Warning |
| Swap | disabled | Blocking |
| cgroup | v2 mounted at `/sys/fs/cgroup` | Blocking |
| Privileges | root | Blocking (relaxed in dry-run mode) |
| Ports free | 6443, 10250, 10259, 10257, 2379, 2380 | Blocking, unless a control plane already exists on the host |

## Pinned versions

One known-good combination, from `internal/cli/versions.go`. Changing a value
there is a deliberate, reviewable act: the point of pinning is that two hosts
installed on two different days end up identical.

| Component | Pinned value |
| --- | --- |
| Kubernetes (control plane and kubelet) | `v1.32.3` |
| `pkgs.k8s.io` repository channel | `v1.32` |
| Debian package version of `kubeadm`, `kubelet`, `kubectl` | `1.32.3-1.1` |
| CNI plugins (`containernetworking/plugins`) | `v1.6.2` |
| `crictl` (cri-tools) | `v1.32.0` |
| Pause / sandbox image | `registry.k8s.io/pause:3.10` |
| Pod CIDR | `10.244.0.0/24` |
| Service CIDR | `10.96.0.0/16` |
| Cluster DNS domain | `cluster.local` |
| Controller namespace | `brunernetes-system` |
| Demo namespace | `brunernetes-demo` |
| Helm release name | `brunernetes` |

`containerd` is installed from the distribution repository rather than pinned to
an upstream release, because the Debian and Ubuntu packages carry the
distribution's own security maintenance. Its version is observed and reported in
`Togliatti.status.containerRuntime` rather than asserted.

Files the installer generates:

| Path | Purpose |
| --- | --- |
| `/etc/modules-load.d/brunernetes.conf` | Kernel modules loaded at boot |
| `/etc/sysctl.d/99-brunernetes.conf` | Required sysctl values |
| `/etc/apt/keyrings/kubernetes-apt-keyring.gpg` | Kubernetes package signing key |
| `/etc/apt/sources.list.d/kubernetes.list` | Kubernetes package repository |
| `/etc/containerd/config.toml` | containerd configuration with `SystemdCgroup = true` |
| `/etc/brunernetes/kubeadm.yaml` | Generated kubeadm configuration, mode `0600` |
| `/etc/cni/net.d/10-brunernetes.conflist` | Minimal CNI chain |
| `$HOME/.kube/config` | Copy of `/etc/kubernetes/admin.conf` for the invoking user |

## Install flow

`brunerctl install --preset garage|showroom|strict|friday`

Nine ordered steps. Every step is safe to repeat: the installer inspects the
current state of the host before it changes anything.

### Step 1 - Preflight

`CollectHostFacts` reads the host without modifying it: `/etc/os-release`,
`/proc/meminfo`, `statfs` on `/`, `/proc/swaps`,
`/sys/fs/cgroup/cgroup.controllers`, `/proc/modules` and `/sys/module/<name>`,
`/proc/sys/...` for each required sysctl, the presence of `containerd`,
`kubeadm`, `kubelet` and `helm` on `PATH`, `/run/containerd/containerd.sock`,
each required CNI plugin binary in `/opt/cni/bin`, a TCP dial against each
required port on `127.0.0.1`, the effective UID, and
`/etc/kubernetes/manifests/kube-apiserver.yaml` to detect an existing cluster.

`Preflight` is a pure function over those facts, so it is unit tested with
synthetic hosts. Results are `OK`, `WARN` or `FAIL`; every `FAIL` carries a
remedy.

| Check | Level on problem | Remedy printed |
| --- | --- | --- |
| Architecture not `amd64` | FAIL | Run Brunernetes on an x86_64 host |
| Distribution not Ubuntu/Debian | WARN | Install packages manually |
| Not root | FAIL | `sudo brunerctl install --preset garage` |
| CPU below 2 cores | FAIL | Provide a larger host |
| Memory below 4 GiB | FAIL | Provide a larger host |
| Free disk below 20 GiB | WARN | Free disk space |
| Swap enabled | FAIL | `sudo swapoff -a` and comment the swap entry in `/etc/fstab` |
| cgroup v2 absent | FAIL | Boot with `systemd.unified_cgroup_hierarchy=1` and reboot |
| `overlay` / `br_netfilter` not loaded | WARN | The installer loads them |
| Required sysctl wrong or unreadable | WARN | The installer corrects it |
| containerd missing, or socket missing | WARN | The installer installs and starts it |
| Control-plane port busy and no existing cluster | FAIL | Stop the process, or uninstall a previous Kubernetes installation |
| `helm` missing | WARN | Install helm and rerun |
| Control plane already present | OK (informational) | `kubeadm init` will be skipped |

Any `FAIL` aborts the installation before a single host change is made:
"Preflight checks failed. The installation was not started. The factory gate
stays closed." `--skip-preflight` continues anyway and is never the recommended
path.

### Step 2 - Kernel modules

Write `overlay` and `br_netfilter` into `/etc/modules-load.d/brunernetes.conf`
(mode `0644`), then `modprobe` each. A module that cannot be loaded produces a
warning rather than a failure, because it may be compiled into the kernel.

### Step 3 - Sysctls

Write the required values into `/etc/sysctl.d/99-brunernetes.conf` in a stable
key order, then `sysctl --system`.

| Key | Value | Why |
| --- | --- | --- |
| `net.bridge.bridge-nf-call-iptables` | `1` | Bridged pod traffic must traverse iptables so Service and portmap rules apply |
| `net.bridge.bridge-nf-call-ip6tables` | `1` | Same for IPv6 |
| `net.ipv4.ip_forward` | `1` | Pod-to-pod and pod-to-host routing |

### Step 4 - Packages

Skipped entirely with `--skip-packages`, and skipped with a warning on a
non-Debian-based distribution. Otherwise:

1. `apt-get update`
2. `apt-get install -y apt-transport-https ca-certificates curl gpg containerd`
3. Install the Kubernetes signing key into
   `/etc/apt/keyrings/kubernetes-apt-keyring.gpg` (mode `0644`)
4. Write `/etc/apt/sources.list.d/kubernetes.list` pointing at
   `https://pkgs.k8s.io/core:/stable:/v1.32/deb/`
5. `apt-get update && apt-get install -y kubelet=1.32.3-1.1 kubeadm=1.32.3-1.1 kubectl=1.32.3-1.1 && apt-mark hold kubelet kubeadm kubectl`

The `apt-mark hold` is what makes the pin survive a routine `apt upgrade`.

A failure at any sub-step aborts with the step named: "package installation
failed while trying to *install the pinned Kubernetes packages*. Check network
access to the distribution and Kubernetes repositories. Parts delivery to the
plant failed."

### Step 5 - containerd configuration

Generate `containerd config default`, rewrite `SystemdCgroup = false` to
`SystemdCgroup = true`, rewrite the sandbox image to
`registry.k8s.io/pause:3.10`, write the result to `/etc/containerd/config.toml`
(mode `0644`), then `systemctl restart containerd` and `systemctl enable
containerd`.

`SystemdCgroup = true` is not optional. The kubelet is configured with
`cgroupDriver: systemd` in the generated kubeadm configuration, and a mismatch
between the two produces a node that appears to start and then fails resource
accounting and eviction in ways that are difficult to diagnose. A failed restart
is fatal: "containerd could not be restarted. Run `systemctl status containerd`
and `journalctl -u containerd` to see why. The machine shop did not start."

### Step 6 - Control plane

Skipped when `/etc/kubernetes/manifests/kube-apiserver.yaml` already exists.
Otherwise write `/etc/brunernetes/kubeadm.yaml` (mode `0600`) and run
`kubeadm init --config /etc/brunernetes/kubeadm.yaml`.

The generated configuration contains four documents:

- `InitConfiguration` - CRI socket `unix:///run/containerd/containerd.sock`,
  node label `bruner.dev/factory=togliatti`, and an optional
  `localAPIEndpoint` when `--api-advertise-address` is given
- `ClusterConfiguration` - `kubernetesVersion: v1.32.3`,
  `clusterName: brunernetes`, `podSubnet: 10.244.0.0/24`,
  `serviceSubnet: 10.96.0.0/16`, `dnsDomain: cluster.local`
- `KubeletConfiguration` - `cgroupDriver: systemd`, `failSwapOn: true`
- `KubeProxyConfiguration` - `mode: iptables`

On success the admin kubeconfig is copied to `$HOME/.kube/config` and chowned to
the invoking user. If that fails the installer tells the user to set
`KUBECONFIG=/etc/kubernetes/admin.conf` instead. A failed `kubeadm init` aborts:
"kubeadm init failed. Run `kubeadm reset -f` before retrying and inspect
`journalctl -u kubelet`. The control-plane building did not open."

### Step 7 - Node preparation and CNI

1. **Remove the control-plane taint** so the single node runs workloads:
   `kubectl taint nodes --all node-role.kubernetes.io/control-plane- --overwrite=false || true`.
   Without this every workload stays `Pending`. The security consequence is in
   [security.md](security.md).
2. **Install the CNI plugins** when any of `bridge`, `loopback`, `host-local`
   or `portmap` is missing from `/opt/cni/bin`: download
   `cni-plugins-linux-amd64-v1.6.2.tgz` from the `containernetworking/plugins`
   release and extract it into `/opt/cni/bin`.
3. **Write `/etc/cni/net.d/10-brunernetes.conflist`** (mode `0644`) - a
   `cniVersion: 1.0.0` chain named `brunernetes`:

```json
{
  "cniVersion": "1.0.0",
  "name": "brunernetes",
  "plugins": [
    {
      "type": "bridge",
      "bridge": "bruner0",
      "isGateway": true,
      "ipMasq": true,
      "hairpinMode": true,
      "ipam": {
        "type": "host-local",
        "ranges": [[{ "subnet": "10.244.0.0/24" }]],
        "routes": [{ "dst": "0.0.0.0/0" }]
      }
    },
    { "type": "portmap", "capabilities": { "portMappings": true } },
    { "type": "loopback" }
  ]
}
```

   `portmap` with `"capabilities": { "portMappings": true }` is what makes the
   loopback `hostPort` binding work at all. Without it the pod runs, the
   ClusterIP Service works, and nothing answers on `127.0.0.1:30080`.
   `hairpinMode` lets a pod reach itself through its own Service address.

4. `systemctl restart kubelet` so the node picks up the network configuration.
   A failure here is a warning: the node may stay `NotReady` until it is
   restarted manually.

`--skip-cni` leaves steps 2 to 4 untouched for hosts that already have a
network configured.

### Step 8 - Controller

Skipped with `--skip-controller`. Otherwise locate the chart - `--chart-path`,
then `charts/brunernetes`, then
`/usr/local/share/brunernetes/charts/brunernetes`, then a path relative to the
executable - and run:

```
KUBECONFIG=/etc/kubernetes/admin.conf helm upgrade --install brunernetes <chart> \
  --namespace brunernetes-system --create-namespace --wait --timeout 5m
```

`helm upgrade --install` is what makes this step idempotent. The chart installs
the four CRDs, the `ServiceAccount`, the `ClusterRole` and binding, and the
controller manager `Deployment`. A missing `helm` binary produces a warning and
the manual command rather than a hard failure.

### Step 9 - Factory resources

Render a `Togliatti` and an `AvtoVAZ` for the selected preset and pipe them to
`kubectl apply -f -`. Preset mapping (`PresetMode`):

| `--preset` | `profile` | `qualityGate` | `requireReadinessProbe` | `requireResourceRequests` |
| --- | --- | --- | --- | --- |
| `garage` (default) | `garage` | `practical` | `false` | `false` |
| `showroom` | `showroom` | `practical` | `false` | `false` |
| `strict` | `strict` | `strict` | `true` | `true` |
| `friday` | `friday` | `practical` | `false` | `false` |

All presets set `rollout: steady`, `shift: day`, `latestTagPolicy: Warn`,
`maxApplications: 1` and `allowPublicExposure: false`. A failure here is a
warning with the instruction to apply `config/samples` manually.

## Demo application

The demo is not deployed by `brunerctl install`. It is a separate, explicit
step so that installing the platform and running a workload stay distinguishable.

```
kubectl create namespace brunernetes-demo
kubectl apply -n brunernetes-demo -f config/samples/bruner.dev_v1alpha1_brunerapp.yaml
```

or, equivalently, `brunerctl app deploy -f config/samples/bruner.dev_v1alpha1_brunerapp.yaml`,
or `make demo`.

The image must already exist in a registry reachable from the node -
Brunernetes never builds images
([ADR 0004](adr/0004-delivery-without-image-build.md)). Build and publish the
Factory Showroom image from `demo/factory-showroom/Dockerfile` with your own
tooling, or side-load it:

```
ctr -n k8s.io images import factory-showroom.tar
```

and set `spec.imagePullPolicy: IfNotPresent` or `Never`.

## Verification

| Check | Command | Expected |
| --- | --- | --- |
| Node | `kubectl get nodes` | One node, `Ready`, roles include `control-plane` |
| Control plane | `kubectl get pods -n kube-system` | `etcd`, `kube-apiserver`, `kube-controller-manager`, `kube-scheduler`, `kube-proxy`, two `coredns` pods all `Running` |
| Taint removed | `kubectl describe node \| grep -i taint` | `Taints: <none>` |
| CRDs | `kubectl get crd \| grep bruner.dev` | `togliattis`, `avtovazs`, `brunerapps`, `konveers` |
| Controller | `kubectl get pods -n brunernetes-system` | The manager pod `Running` and ready |
| Factory resources | `kubectl get togliatti,avtovaz` | `togliatti` and `avtovaz` present |
| Application | `kubectl get brunerapp -n brunernetes-demo` | Stage `Showroom`, `Available` `True`, local endpoint printed |
| Endpoint | `curl -fsS http://127.0.0.1:30080/healthz` | HTTP 200 |
| Off-host isolation | `curl --max-time 3 http://<node-ip>:30080/` from another host | Connection refused or timeout |
| Summary | `brunerctl status` and `brunerctl diagnose` | No blocking findings |

The off-host check is the one that proves the product guarantee, so it belongs
in the verification list rather than in a footnote.

## Uninstall flow

`brunerctl uninstall [--keep-packages] [--purge] [--dry-run]`

Six ordered steps, the inverse of the install flow wherever an inverse exists.
Package removal and container runtime removal are opt-out and opt-in
respectively, because both may be used by something else on the host. Every step
tolerates failure and continues, because a partially installed host must still
be cleanable.

| Step | Action | Notes |
| --- | --- | --- |
| 1 | `helm uninstall brunernetes -n brunernetes-system --ignore-not-found` | Removes the controller. Whether the chart also removes the CRDs - and therefore the CRs - depends on the chart's CRD retention policy; see [open-questions.md](open-questions.md) |
| 2 | `kubectl delete namespace brunernetes-system brunernetes-demo --ignore-not-found --wait=false` | Deleting the demo namespace deletes the `BrunerApp`, which garbage-collects its Deployment and Service |
| 3 | `kubeadm reset -f --cri-socket unix:///run/containerd/containerd.sock` | Stops the static pods, drains etcd, undoes the kubelet configuration |
| 4 | Remove `/etc/cni/net.d/10-brunernetes.conflist`, `/etc/sysctl.d/99-brunernetes.conf`, `/etc/modules-load.d/brunernetes.conf`, `/etc/brunernetes`, `/etc/cni/net.d`, `/var/lib/etcd`, `/etc/kubernetes`; then flush iptables (`filter`, `nat`, `mangle`, and `-X`) | The iptables flush is blunt: it removes **all** rules in those tables, not only the ones Kubernetes created. On a host with its own firewall rules, skip this and reboot instead |
| 5 | `apt-mark unhold` then `apt-get purge -y kubelet kubeadm kubectl`, remove the repository and keyring, `apt-get autoremove -y` | Skipped with `--keep-packages` |
| 6 | `systemctl stop containerd`, `apt-get purge -y containerd`, `rm -rf /var/lib/containerd /etc/containerd` | Only with `--purge`. Without it, containerd and every cached image stay |

Closing message: "Brunernetes and the single-node control plane are removed.
Reboot the host to clear any remaining network state. The factory floor is swept
and the lights are off."

**What uninstall deliberately does not undo:** it does not re-enable swap, does
not restore a previous `/etc/containerd/config.toml`, does not remove
`$HOME/.kube/config`, and does not remove the `bruner0` bridge interface - a
reboot clears the last one. These are stated rather than attempted, because a
half-successful restoration of unrelated host state is worse than no
restoration.

## Commands requiring `sudo`

The installer must run as root; the preflight check fails otherwise. This table
enumerates every privileged operation and the reason it needs privilege, so the
`sudo` surface is reviewable rather than implicit. See
[security.md](security.md) for the risk assessment.

| Step | Command or operation | Why root is required |
| --- | --- | --- |
| 2 | Write `/etc/modules-load.d/brunernetes.conf` | Writing under `/etc` |
| 2 | `modprobe overlay`, `modprobe br_netfilter` | Loading kernel modules is a privileged kernel operation |
| 3 | Write `/etc/sysctl.d/99-brunernetes.conf` | Writing under `/etc` |
| 3 | `sysctl --system` | Changing kernel parameters |
| 4 | `apt-get update`, `apt-get install`, `apt-mark hold` | Package management writes to `/var/lib/dpkg` and `/usr` |
| 4 | Write `/etc/apt/keyrings/kubernetes-apt-keyring.gpg`, `/etc/apt/sources.list.d/kubernetes.list` | Writing under `/etc`; trusting a package signing key |
| 5 | Write `/etc/containerd/config.toml` | Writing under `/etc` |
| 5 | `systemctl restart containerd`, `systemctl enable containerd` | Controlling a system service |
| 6 | Write `/etc/brunernetes/kubeadm.yaml` (mode `0600`) | Writing under `/etc`; the file may contain the API advertise address |
| 6 | `kubeadm init` | Writes `/etc/kubernetes`, generates the cluster PKI, starts static pods, configures and starts the kubelet |
| 6 | `cp /etc/kubernetes/admin.conf $HOME/.kube/config` | Reading a root-owned `0600` credential |
| 7 | `kubectl taint nodes ...` with `/etc/kubernetes/admin.conf` | Reading the root-owned admin kubeconfig |
| 7 | Extract CNI plugins into `/opt/cni/bin` | Writing under `/opt` |
| 7 | Write `/etc/cni/net.d/10-brunernetes.conflist` | Writing under `/etc` |
| 7 | `systemctl restart kubelet` | Controlling a system service |
| 8 | `helm upgrade --install` with `KUBECONFIG=/etc/kubernetes/admin.conf` | Reading the root-owned admin kubeconfig |
| 9 | `kubectl apply` with `KUBECONFIG=/etc/kubernetes/admin.conf` | Reading the root-owned admin kubeconfig |
| Uninstall 1–2 | `helm uninstall`, `kubectl delete namespace` | Reading the root-owned admin kubeconfig |
| Uninstall 3 | `kubeadm reset -f` | Stopping static pods, removing `/etc/kubernetes`, reconfiguring the kubelet |
| Uninstall 4 | `rm -rf` under `/etc` and `/var/lib`; `iptables -F`, `-t nat -F`, `-t mangle -F`, `-X` | Removing root-owned files; modifying the host firewall |
| Uninstall 5 | `apt-mark unhold`, `apt-get purge`, `apt-get autoremove` | Package management |
| Uninstall 6 | `systemctl stop containerd`, `apt-get purge containerd`, `rm -rf /var/lib/containerd /etc/containerd` | Controlling a service; removing root-owned state |

Steps 7 to 9 and uninstall steps 1 to 2 need root only to *read*
`/etc/kubernetes/admin.conf`. A user whose own kubeconfig already has
cluster-admin can run those without privilege.

Read-only commands need no privilege: `brunerctl status`,
`brunerctl factory status`, `brunerctl app list`, `brunerctl app inspect`,
`brunerctl events` and `brunerctl diagnose` all use the user's kubeconfig.

## Dry-run behaviour

`--dry-run` on both `install` and `uninstall` swaps the real `ExecRunner` for a
`DryRunner`. The interface is identical, so the flow, the ordering and the
branching are exactly the same as a real run; only the effects are replaced by
printed intentions:

| Operation | Dry-run output |
| --- | --- |
| `Run(name, args...)` | The rendered command line |
| `RunWithInput(stdin, ...)` | The command line plus `(with N bytes on stdin)` |
| `WriteFile(path, content, mode)` | `write <path> (N bytes, mode 0644)` |
| `RemoveAll(path)` | `remove <path>` |

Three consequences worth knowing:

1. **The root check is relaxed.** A dry run sets `facts.Root = true` before
   evaluating preflight, so a non-root operator can produce a useful plan
   without the report looking broken. Every other preflight check runs against
   the real host, so a dry run is a genuine host assessment.
2. **Commands that return output return nothing.** `containerd config default`
   produces an empty string under the dry runner, so the containerd step reports
   the intent to write the file without being able to show its content. This is
   the one place where dry-run output is less informative than the real run.
3. **The mode is stated up front and at the end.** The summary block shows
   `Mode: dry run, no host change is applied`, and the closing line is "Dry run
   finished. No change was applied to this host. The plan is on the desk, the
   line has not moved." (uninstall: "The decommissioning plan is on the desk,
   the line is untouched.")

A dry run is the recommended first command on any new host:

```
sudo brunerctl install --preset garage --dry-run
```

## Idempotency

Rerunning the installer on an installed host is expected and safe:

- Preflight recognises an existing control plane and downgrades the busy-port
  failure to an informational pass.
- Kernel module, sysctl and configuration files are rewritten with identical
  content.
- `kubeadm init` is skipped entirely when
  `/etc/kubernetes/manifests/kube-apiserver.yaml` exists.
- The taint removal uses `--overwrite=false` and tolerates "not found".
- CNI plugins are downloaded only when a binary is missing.
- `helm upgrade --install` converges the release.
- `kubectl apply` converges the factory resources.

## Related documents

- [ADR 0001: Single node on upstream Kubernetes](adr/0001-single-node-upstream-kubernetes.md)
- [ADR 0003: Localhost-only exposure](adr/0003-localhost-only-exposure.md)
- [ADR 0004: Delivery without image build](adr/0004-delivery-without-image-build.md)
- [architecture.md](architecture.md)
- [security.md](security.md)
- [repository-structure.md](repository-structure.md)
