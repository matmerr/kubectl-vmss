package get

import (
	"context"
	"fmt"

	"github.com/matmerr/kubectl-vmss/pkg/vmss"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

type getPodsOptions struct {
	namespace string
	node      string
	allNs     bool

	runner  vmss.Runner
	streams genericclioptions.IOStreams
}

// NewCmdGetPods returns a cobra command for "kubectl vmss get pods".
func NewCmdGetPods(streams genericclioptions.IOStreams) *cobra.Command {
	o := &getPodsOptions{
		namespace: "kube-system",
		streams:   streams,
	}

	cmd := &cobra.Command{
		Use:   "pods <node>",
		Short: "List pods/containers on a node via crictl",
		Long:  "List running containers on an AKS node using crictl via VMSS run-command. Useful when the API server cannot reach the node.",
		Example: `  # List pods on a node
  kubectl vmss get po aks-nodepool1-vmss000000

  # Show all containers (including exited)
  kubectl vmss get pods aks-nodepool1-vmss000000 -a`,
		Aliases: []string{"pod", "po"},
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			o.node = args[0]
			if o.runner == nil {
				o.runner = vmss.NewDefaultRunner()
			}
			return o.Run(cmd.Context())
		},
	}

	cmd.Flags().StringVarP(&o.namespace, "namespace", "n", o.namespace, "Namespace for pod lookup")
	cmd.Flags().BoolVarP(&o.allNs, "all", "a", false, "Show all containers including exited")

	return cmd
}

func (o *getPodsOptions) Run(ctx context.Context) error {
	node := o.node

	info, err := o.runner.ResolveVMSS(ctx, node)
	if err != nil {
		return err
	}

	var script string
	if o.allNs {
		script = "crictl ps -a -o table"
	} else {
		script = "crictl ps -o table"
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
