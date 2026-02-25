package run

import (
	"context"
	"fmt"

	"github.com/matmerr/kubectl-vmss/pkg/vmss"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

type runOptions struct {
	namespace string
	node      string
	pod       string
	command   string

	runner  vmss.Runner
	streams genericclioptions.IOStreams
}

// NewCmdRun returns a cobra command for "kubectl vmss run".
func NewCmdRun(streams genericclioptions.IOStreams) *cobra.Command {
	o := &runOptions{
		namespace: "kube-system",
		streams:   streams,
	}

	cmd := &cobra.Command{
		Use:   "run <node> <command>",
		Short: "Run an arbitrary command on a node",
		Long:  "Run an arbitrary shell command on an AKS node via VMSS run-command.",
		Example: `  # Run a command on a specific node
  kubectl vmss run aks-nodepool1-vmss000000 "journalctl -u kubelet -n 50"

  # Run a command on a pod's node
  kubectl vmss run --pod cilium-6jnvz "journalctl -u kubelet -n 20"`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if o.pod != "" {
				// When --pod is used, the single arg is the command
				if len(args) != 1 {
					return fmt.Errorf("when using --pod, provide exactly one argument: the command")
				}
				o.command = args[0]
			} else {
				if len(args) < 2 {
					return fmt.Errorf("requires <node> <command> (or use --pod <pod> <command>)")
				}
				o.node = args[0]
				o.command = args[1]
			}
			if o.runner == nil {
				o.runner = vmss.NewDefaultRunner()
			}
			return o.Run(cmd.Context())
		},
	}

	cmd.Flags().StringVarP(&o.namespace, "namespace", "n", o.namespace, "Namespace for pod lookup")
	cmd.Flags().StringVar(&o.pod, "pod", "", "Resolve node from this pod")

	return cmd
}

// Run executes the run command.
func (o *runOptions) Run(ctx context.Context) error {
	node := o.node

	if o.pod != "" {
		var err error
		node, err = o.runner.ResolveNodeFromPod(ctx, o.namespace, o.pod)
		if err != nil {
			return err
		}
	}

	info, err := o.runner.ResolveVMSS(ctx, node)
	if err != nil {
		return err
	}

	fmt.Fprintf(o.streams.ErrOut, "Running on %s/%s...\n", info.VMSSName, info.InstanceID)
	result, err := o.runner.RunCommand(ctx, info, o.command)
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
