package get

import (
	"context"
	"fmt"

	"github.com/matmerr/kubectl-vmss/pkg/vmss"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

type getNetnsOptions struct {
	node string

	runner  vmss.Runner
	streams genericclioptions.IOStreams
}

// NewCmdGetNetns returns a cobra command for "kubectl vmss get netns".
func NewCmdGetNetns(streams genericclioptions.IOStreams) *cobra.Command {
	o := &getNetnsOptions{
		streams: streams,
	}

	cmd := &cobra.Command{
		Use:   "netns <node>",
		Short: "List network namespaces on a node",
		Long:  "Query network namespaces on an AKS node via VMSS run-command. Shows output from lsns and ip netns.",
		Example: `  # List network namespaces on a node
  kubectl vmss get netns aks-nodepool1-vmss000000`,
		Aliases: []string{"networknamespaces", "nns"},
		Args:    cobra.ExactArgs(1),
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

func (o *getNetnsOptions) Run(ctx context.Context) error {
	node := o.node

	info, err := o.runner.ResolveVMSS(ctx, node)
	if err != nil {
		return err
	}

	// List all net namespaces using lsns, plus named namespaces via ip netns
	script := `echo "=== Network Namespaces (lsns) ===" && lsns -t net -o NS,PID,USER,COMMAND 2>/dev/null || true && echo "" && echo "=== Named Network Namespaces (ip netns) ===" && ip netns list 2>/dev/null || echo "(none)"`

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
