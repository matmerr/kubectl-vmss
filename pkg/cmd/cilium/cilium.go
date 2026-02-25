package cilium

import (
	"context"
	"fmt"
	"strings"

	"github.com/matmerr/kubectl-vmss/pkg/vmss"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

type ciliumOptions struct {
	namespace string
	pod       string
	args      []string

	runner  vmss.Runner
	streams genericclioptions.IOStreams
}

// NewCmdCilium returns a cobra command for "kubectl vmss cilium".
// Usage: kubectl vmss cilium <pod> [cilium-args...]
// All args after the pod name are passed directly to the cilium CLI on the node.
//
// The script mounts the cilium container image via containerd (ctr), then uses
// nsenter to run the binary inside the pod's network namespace.  This works
// even when the cilium pod is in CrashLoopBackOff because the binary comes
// from the image layers, not from a running container.
func NewCmdCilium(streams genericclioptions.IOStreams) *cobra.Command {
	o := &ciliumOptions{
		namespace: "kube-system",
		streams:   streams,
	}

	cmd := &cobra.Command{
		Use:   "cilium <pod> [flags] [-- cilium-args...]",
		Short: "Run cilium CLI commands on a pod's node",
		Long: `Run the cilium CLI directly on the AKS node where a cilium pod is scheduled.

This works even when the pod is in CrashLoopBackOff.  The plugin:
  1. Finds the pod sandbox (always alive) and its PID.
  2. Identifies the cilium container image from crictl inspect.
  3. Mounts the image via 'ctr -n k8s.io images mount' to get the binary.
  4. Runs the cilium CLI in the pod's network namespace via nsenter.
  5. Unmounts the image after the command finishes.

All arguments after the pod name (or after --) are passed to the cilium binary.`,
		Example: `  # Check cilium status on the node
  kubectl vmss cilium cilium-6jnvz status

  # List cilium endpoints
  kubectl vmss cilium cilium-6jnvz endpoint list

  # Get BPF policy for an endpoint
  kubectl vmss cilium cilium-6jnvz bpf policy get 1234

  # Run with verbose output
  kubectl vmss cilium cilium-6jnvz status --verbose`,
		DisableFlagParsing: false,
		Args:               cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			o.pod = args[0]
			o.args = args[1:]
			if o.runner == nil {
				o.runner = vmss.NewDefaultRunner()
			}
			return o.Run(cmd.Context())
		},
	}

	cmd.Flags().StringVarP(&o.namespace, "namespace", "n", o.namespace, "Namespace for pod lookup")

	return cmd
}

// buildCiliumScript generates the shell script that:
//  1. Finds the cilium pod sandbox PID (always alive, even in CrashLoopBackOff).
//  2. Discovers the cilium container image via crictl inspect.
//  3. Mounts the image with ctr to get the cilium binary.
//  4. Runs the binary via nsenter in the pod's network namespace.
//  5. Cleans up the mount.
func buildCiliumScript(podName, ciliumArgs string) string {
	return fmt.Sprintf(`set -e

POD_NAME=%q
CILIUM_ARGS=%q
MNT="/tmp/cilium-rootfs-$$"

# --- Step 1: Find the cilium pod sandbox PID ---
SANDBOX_ID=$(crictl pods --name "$POD_NAME" -q | head -1)
if [ -z "$SANDBOX_ID" ]; then
  echo "Error: no sandbox found for pod $POD_NAME" >&2
  exit 1
fi
SANDBOX_PID=$(crictl inspectp "$SANDBOX_ID" 2>/dev/null \
  | python3 -c "import sys,json; print(json.load(sys.stdin)['info']['pid'])" 2>/dev/null || true)
if [ -z "$SANDBOX_PID" ] || [ "$SANDBOX_PID" = "0" ]; then
  echo "Error: could not determine sandbox PID for pod $POD_NAME" >&2
  exit 1
fi

# --- Step 2: Find the cilium container image ---
CID=$(crictl ps --name cilium-agent -q | head -1)
if [ -z "$CID" ]; then
  CID=$(crictl ps -a --name cilium-agent -q | head -1)
fi
if [ -z "$CID" ]; then
  echo "Error: no cilium-agent container found" >&2
  exit 1
fi
IMAGE=$(crictl inspect "$CID" 2>/dev/null \
  | python3 -c "import sys,json; d=json.load(sys.stdin); print(d['info']['config']['image']['image'])" 2>/dev/null || true)
if [ -z "$IMAGE" ]; then
  # fallback: use the imageRef from the status
  IMAGE=$(crictl inspect "$CID" 2>/dev/null \
    | python3 -c "import sys,json; d=json.load(sys.stdin); print(d['status']['imageRef'])" 2>/dev/null || true)
fi
if [ -z "$IMAGE" ]; then
  echo "Error: could not determine cilium container image" >&2
  exit 1
fi

# --- Step 3: Mount the container image ---
cleanup() { ctr -n k8s.io images unmount "$MNT" >/dev/null 2>&1 || true; rmdir "$MNT" 2>/dev/null || true; }
trap cleanup EXIT
mkdir -p "$MNT"
ctr -n k8s.io images mount "$IMAGE" "$MNT" >/dev/null 2>&1
if [ ! -x "$MNT/usr/bin/cilium-dbg" ] && [ ! -x "$MNT/usr/bin/cilium" ]; then
  echo "Error: cilium binary not found in image $IMAGE" >&2
  exit 1
fi

# Prefer cilium-dbg (newer cilium) over cilium
CILIUM_BIN="$MNT/usr/bin/cilium-dbg"
if [ ! -x "$CILIUM_BIN" ]; then
  CILIUM_BIN="$MNT/usr/bin/cilium"
fi

# --- Step 4: nsenter into the pod network namespace and run ---
nsenter -t "$SANDBOX_PID" -n -- "$CILIUM_BIN" $CILIUM_ARGS
`, podName, ciliumArgs)
}

// Run executes the cilium command on the node.
func (o *ciliumOptions) Run(ctx context.Context) error {
	fmt.Fprintf(o.streams.ErrOut, "Resolving node from pod %s/%s...\n", o.namespace, o.pod)
	node, err := o.runner.ResolveNodeFromPod(ctx, o.namespace, o.pod)
	if err != nil {
		return err
	}
	fmt.Fprintf(o.streams.ErrOut, "Pod %s/%s is on node: %s\n", o.namespace, o.pod, node)

	info, err := o.runner.ResolveVMSS(ctx, node)
	if err != nil {
		return err
	}

	ciliumArgs := strings.Join(o.args, " ")
	script := buildCiliumScript(o.pod, ciliumArgs)

	fmt.Fprintf(o.streams.ErrOut, "Running cilium %s on %s/%s...\n", ciliumArgs, info.VMSSName, info.InstanceID)
	result, err := o.runner.RunCommand(ctx, info, script)
	if err != nil {
		return err
	}

	if result.Stdout != "" {
		fmt.Fprintln(o.streams.Out, result.Stdout)
	}
	if result.Stderr != "" {
		fmt.Fprintln(o.streams.ErrOut, result.Stderr)
	}
	return nil
}
