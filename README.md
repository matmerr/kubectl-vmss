# kubectl-vmss

A kubectl plugin to run commands on AKS nodes via Azure VMSS run-command.

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
kubectl vmss acn logs  <node>                                  # full CNI/CNS log files
kubectl vmss acn logs  <node> --tail 500                       # last 500 lines per file
kubectl vmss acn state <node>                                  # CNI/CNS state & config files

# Cilium CLI (works even in CrashLoopBackOff)
kubectl vmss cilium <pod> status
kubectl vmss cilium <pod> endpoint list
kubectl vmss cilium <pod> version
```

## How It Works

1. Given a pod name, uses `kubectl` to find the node it is scheduled on.
2. Reads the node's `spec.providerID` (e.g. `azure:///subscriptions/.../virtualMachineScaleSets/<vmss>/virtualMachines/<id>`) to extract the VMSS coordinates.
3. Runs commands on the node via `az vmss run-command invoke`, so you can inspect the host even when the API server can't reach the node or when pods are in CrashLoopBackOff.

For the `cilium` subcommand, the plugin mounts the cilium container image via `ctr`, then uses `nsenter` to run the binary inside the pod's network namespace — no running container required.

## Prerequisites

- `kubectl` configured with cluster access
- `az` CLI authenticated with access to the cluster's VMSS resources

## Install

Download the binary for your platform from the [latest release](https://github.com/matmerr/kubectl-vmss/releases) or build from source:

```bash
go install github.com/matmerr/kubectl-vmss/cmd/kubectl-vmss@latest
```

Move the binary to your PATH as `kubectl-vmss` so kubectl discovers it as a plugin:

```bash
mv $(go env GOPATH)/bin/kubectl-vmss /usr/local/bin/kubectl-vmss
```

### Build from Source

```bash
git clone https://github.com/matmerr/kubectl-vmss.git
cd kubectl-vmss
make install
```

### Docker

```bash
docker build -t kubectl-vmss .
```

## Commands

```
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

## Project Structure

```
Makefile                        Build, test, install, dist targets
cmd/kubectl-vmss/main.go        Entry point
pkg/cmd/root.go                 Root cobra command wiring
pkg/cmd/logs/logs.go            logs subcommand
pkg/cmd/exec/exec.go            exec subcommand
pkg/cmd/run/run.go              run subcommand
pkg/cmd/get/get.go              get parent command (pods, netns)
pkg/cmd/get/pods.go             get pods subcommand
pkg/cmd/get/netns.go            get netns subcommand
pkg/cmd/get/azcni.go            get azcni-logs / azcni-state (deprecated aliases)
pkg/cmd/acn/acn.go              acn parent command
pkg/cmd/acn/logs.go             acn logs subcommand
pkg/cmd/acn/state.go            acn state subcommand
pkg/cmd/cilium/cilium.go        cilium subcommand
pkg/version/version.go          Version variables (set via ldflags)
pkg/vmss/vmss.go                VMSS runner (kubectl/az shell-out, providerID parsing)
pkg/vmss/vmss_test.go           Unit tests (providerID parsing, output parsing)
test/integration/               Integration tests with mock runner
```
