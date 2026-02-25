package exec

import (
	"context"
	"fmt"

	"github.com/matmerr/kubectl-vmss/pkg/vmss"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

type execOptions struct {
	namespace string
	node      string
	pod       string
	command   string

	runner  vmss.Runner
	streams genericclioptions.IOStreams
}

// NewCmdExec returns a cobra command for "kubectl vmss exec".
func NewCmdExec(streams genericclioptions.IOStreams) *cobra.Command {
	o := &execOptions{
		namespace: "kube-system",
		streams:   streams,
	}

	cmd := &cobra.Command{
		Use:   "exec <pod> [command]",
		Short: "Run a command on the pod's node",
		Long:  "Run a command on the AKS node where a pod is scheduled, via VMSS run-command.",
		Example: `  # Get basic node info
  kubectl vmss exec cilium-6jnvz

  # Run a specific command on the node
  kubectl vmss exec cilium-6jnvz "cat /etc/resolv.conf"`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			o.pod = args[0]
			if len(args) > 1 {
				o.command = args[1]
			}
			if o.runner == nil {
				o.runner = vmss.NewDefaultRunner()
			}
			return o.Run(cmd.Context())
		},
	}

	cmd.Flags().StringVarP(&o.namespace, "namespace", "n", o.namespace, "Namespace for pod lookup")
	cmd.Flags().StringVar(&o.node, "node", "", "Target a node directly instead of a pod")

	return cmd
}

// Run executes the exec command.
func (o *execOptions) Run(ctx context.Context) error {
	node := o.node

	if o.pod != "" && node == "" {
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

	cmd := o.command
	if cmd == "" {
		cmd = "echo 'Connected to node. Run commands:'; uname -a; echo '---'; ps aux | head -20"
	}

	fmt.Fprintf(o.streams.ErrOut, "Running on %s/%s...\n", info.VMSSName, info.InstanceID)
	result, err := o.runner.RunCommand(ctx, info, cmd)
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
