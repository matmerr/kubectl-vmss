package acn

import (
	"context"
	"fmt"

	"github.com/matmerr/kubectl-vmss/pkg/vmss"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

type acnStateOptions struct {
	node string

	runner  vmss.Runner
	streams genericclioptions.IOStreams
}

// NewCmdACNState returns a cobra command for "kubectl vmss acn state".
func NewCmdACNState(streams genericclioptions.IOStreams) *cobra.Command {
	o := &acnStateOptions{
		streams: streams,
	}

	cmd := &cobra.Command{
		Use:   "state <node>",
		Short: "Get Azure CNI state files from a node",
		Long:  "Retrieve Azure CNI / Azure CNS state files (JSON) from an AKS node via VMSS run-command.",
		Example: `  # Get Azure CNI state from a node
  kubectl vmss acn state aks-nodepool1-vmss000000`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			o.node = args[0]
			if o.runner == nil {
				o.runner = vmss.NewDefaultRunner()
			}
			return o.Run(cmd.Context())
		},
	}

	return cmd
}

func (o *acnStateOptions) Run(ctx context.Context) error {
	node := o.node

	info, err := o.runner.ResolveVMSS(ctx, node)
	if err != nil {
		return err
	}

	script := `for f in \
  /etc/cni/net.d/10-azure.conflist \
  /etc/cni/net.d/05-cilium.conflist \
  /etc/cni/net.d/05-cilium.conf; do
  if [ -f "$f" ]; then
    echo "=== $f ==="
    cat "$f"
    echo ""
  fi
done
# Azure CNI downloads (may contain multiple versions)
for f in $(find /opt/cni/downloads/ -name '*.conflist' -o -name '*.json' 2>/dev/null); do
  echo "=== $f ==="
  cat "$f"
  echo ""
done
# Azure CNS / azure-vnet state directories
for d in /var/run/azure-cns /var/run/azure-vnet /var/lib/azure-cns /opt/cns; do
  if [ -d "$d" ]; then
    echo "=== $d ==="
    find "$d" -type f | while read sf; do
      echo "--- $sf ---"
      cat "$sf"
      echo ""
    done
  fi
done`

	fmt.Fprintf(o.streams.ErrOut, "Running on %s/%s...\n", info.VMSSName, info.InstanceID)
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
