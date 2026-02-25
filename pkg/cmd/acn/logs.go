package acn

import (
	"context"
	"fmt"

	"github.com/matmerr/kubectl-vmss/pkg/vmss"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

type acnLogsOptions struct {
	node string
	tail int

	runner  vmss.Runner
	streams genericclioptions.IOStreams
}

// NewCmdACNLogs returns a cobra command for "kubectl vmss acn logs".
func NewCmdACNLogs(streams genericclioptions.IOStreams) *cobra.Command {
	o := &acnLogsOptions{
		streams: streams,
	}

	cmd := &cobra.Command{
		Use:   "logs <node>",
		Short: "Get Azure CNI log files from a node",
		Long:  "Retrieve Azure CNI / Azure CNS log files from an AKS node via VMSS run-command.",
		Example: `  # Get Azure CNI logs from a node
  kubectl vmss acn logs aks-nodepool1-vmss000000

  # Show last 500 lines
  kubectl vmss acn logs aks-nodepool1-vmss000000 --tail 500`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			o.node = args[0]
			if o.runner == nil {
				o.runner = vmss.NewDefaultRunner()
			}
			return o.Run(cmd.Context())
		},
	}

	cmd.Flags().IntVar(&o.tail, "tail", 0, "Number of log lines to show per file (0 = all)")

	return cmd
}

func (o *acnLogsOptions) Run(ctx context.Context) error {
	node := o.node

	info, err := o.runner.ResolveVMSS(ctx, node)
	if err != nil {
		return err
	}

	var script string
	if o.tail > 0 {
		script = fmt.Sprintf(`for f in /var/log/azure-vnet.log /var/log/azure-vnet-ipam.log /var/log/azure-vnet-ipamv2.log /var/log/azure-vnet-telemetry.log /var/log/azure-cnimonitor.log /var/log/azure-cns/azure-cns.log; do
  if [ -f "$f" ]; then
    echo "=== $f (last %d lines) ==="
    tail -n %d "$f"
    echo ""
  fi
done
echo "=== azure-cns (journalctl, last %d lines) ==="
journalctl -u azure-cns -n %d --no-pager 2>/dev/null || echo "(not available)"`, o.tail, o.tail, o.tail, o.tail)
	} else {
		script = `for f in /var/log/azure-vnet.log /var/log/azure-vnet-ipam.log /var/log/azure-vnet-ipamv2.log /var/log/azure-vnet-telemetry.log /var/log/azure-cnimonitor.log /var/log/azure-cns/azure-cns.log; do
  if [ -f "$f" ]; then
    echo "=== $f ==="
    cat "$f"
    echo ""
  fi
done
echo "=== azure-cns (journalctl) ==="
journalctl -u azure-cns --no-pager 2>/dev/null || echo "(not available)"`
	}

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
