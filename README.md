# kubectl-vmss

A kubectl plugin to run commands on AKS nodes via Azure VMSS run-command.

## Install

### Krew (recommended)

```bash
kubectl krew install --manifest-url=https://github.com/matmerr/kubectl-vmss/releases/latest/download/vmss.yaml
```

## Usage

```bash
# Get container logs (even from crashing pods)
kubectl vmss logs <pod>
kubectl vmss logs <pod> --tail 50
kubectl vmss logs <pod> --previous

# Run a command on a pod's node
kubectl vmss exec <pod> "cat /etc/resolv.conf"

# Run an arbitrary command on a node
kubectl vmss run <node> "journalctl -u kubelet -n 50"
kubectl vmss run --pod <pod> "journalctl -u kubelet"          # resolve node from pod

# List pods / network namespaces on a node
kubectl vmss get po <node>
kubectl vmss get po <node> -a                                  # include exited containers
kubectl vmss get netns <node>

# Azure CNI / CNS diagnostics
kubectl vmss acn logs <node>                                  # full CNI/CNS log files
kubectl vmss acn logs <node> --tail 500                       # last 500 lines per file
kubectl vmss acn state <node>                                  # CNI/CNS state & config files

# Cilium CLI (works even in CrashLoopBackOff)
kubectl vmss cilium <pod> status
kubectl vmss cilium <pod> endpoint list
kubectl vmss cilium <pod> version
```

## Examples

#### Fetch container logs

```bash
$ kubectl vmss logs my-pod --tail 5
Resolving VMSS info from node: aks-nodepool1-12345678-vmss000000
  Subscription:  xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
  ResourceGroup: MC_my-rg_my-cluster_eastus
  VMSS:          aks-nodepool1-12345678-vmss
  Instance:      0
Running on aks-nodepool1-12345678-vmss/0...
2026-02-25T01:00:00.000Z level=info msg="starting health check"
2026-02-25T01:00:01.000Z level=info msg="endpoint regeneration complete"
2026-02-25T01:00:02.000Z level=info msg="policy resolved"
2026-02-25T01:00:03.000Z level=info msg="BPF program attached"
2026-02-25T01:00:04.000Z level=info msg="node ready"
```

#### Run a command on a node

```bash
$ kubectl vmss run aks-nodepool1-12345678-vmss000000 "uptime"
Resolving VMSS info from node: aks-nodepool1-12345678-vmss000000
  Subscription:  xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
  ResourceGroup: MC_my-rg_my-cluster_eastus
  VMSS:          aks-nodepool1-12345678-vmss
  Instance:      0
Running on aks-nodepool1-12345678-vmss/0...
 01:00:00 up 10 days,  3:00,  0 users,  load average: 0.50, 0.40, 0.35
```

#### List containers on a node

```bash
$ kubectl vmss get pods aks-nodepool1-12345678-vmss000000
Resolving VMSS info from node: aks-nodepool1-12345678-vmss000000
  Subscription:  xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
  ResourceGroup: MC_my-rg_my-cluster_eastus
  VMSS:          aks-nodepool1-12345678-vmss
  Instance:      0
Running on aks-nodepool1-12345678-vmss/0...
CONTAINER           IMAGE               CREATED             STATE               NAME                  POD ID
a1b2c3d4e5f6        quay.io/cilium...   2 hours ago         Running             cilium-agent          abcdef123456
f6e5d4c3b2a1        mcr.microsoft...    2 hours ago         Running             azure-cns             123456abcdef
```

## Commands

```bash
kubectl vmss logs  <pod>                         # Container logs via crictl
kubectl vmss exec  <pod> [command]               # Run a command on the pod's node
kubectl vmss run   <node> <command>              # Run a command on a node
kubectl vmss get pods  <node>                    # List pods/containers              (aliases: pod, po)
kubectl vmss get netns <node>                    # List network namespaces            (aliases: networknamespaces, nns)
kubectl vmss acn logs  <node>                    # Azure CNI / CNS log files
kubectl vmss acn state <node>                    # Azure CNI / CNS state & config files
kubectl vmss cilium <pod> [args...]              # Cilium CLI in the pod's netns
kubectl vmss version                             # Print version info
```

| Command     | Description                                                                                                                                                  |
| ----------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `logs`      | Get container logs from the node via `crictl`. Resolves pod → node → VMSS automatically.                                                                     |
| `exec`      | Run a command on a pod's node. If no command is given, prints basic host info (`uname`, top processes).                                                      |
| `run`       | Run an arbitrary shell command on a node. Node is positional; use `--pod` to resolve from a pod instead.                                                     |
| `get pods`  | List running pods/containers on a node via `crictl`.                                                                                                         |
| `get netns` | List network namespaces on a node via `lsns` and `ip netns`.                                                                                                 |
| `acn logs`  | Show Azure CNI / CNS log files (`/var/log/azure-cns/`, `/var/log/azure-vnet*.log`) and journald entries from a node.                                         |
| `acn state` | Dump Azure CNI / CNS state and config files (`/var/run/azure-cns/`, `/etc/cni/net.d/`, `/opt/cni/downloads/`) from a node.                                   |
| `cilium`    | Run the `cilium` CLI in a pod's network namespace. Mounts the container image via `ctr` and uses `nsenter` — works even when the pod is in CrashLoopBackOff. |
| `version`   | Print version, git commit, and build date.                                                                                                                   |

### Options

| Flag              | Applies to                                  | Description                                | Default               |
| ----------------- | ------------------------------------------- | ------------------------------------------ | --------------------- |
| `-n, --namespace` | `logs`, `exec`, `run`, `cilium`, `get pods` | Namespace for pod lookup                   | `kube-system`         |
| `--node`          | `logs`, `exec`                              | Target a specific node directly            | _(resolved from pod)_ |
| `--pod`           | `run`                                       | Resolve node from this pod                 |                       |
| `--tail`          | `logs`, `acn logs`                          | Number of log lines to show (0 = all)      | `0` (all)             |
| `--previous`      | `logs`                                      | Show logs from previous container instance | `false`               |
| `-a, --all`       | `get pods`                                  | Show all containers including exited       | `false`               |

## How It Works

1. Given a pod name, uses `kubectl` to find the node it is scheduled on.
2. Reads the node's `spec.providerID` to extract the VMSS coordinates (subscription, resource group, scale set, instance ID).
3. Runs commands on the node via `az vmss run-command invoke`, so you can inspect the host even when the API server can't reach the node or when pods are in CrashLoopBackOff.

For the `cilium` subcommand, the plugin mounts the cilium container image via `ctr`, then uses `nsenter` to run the binary inside the pod's network namespace — no running container required.
