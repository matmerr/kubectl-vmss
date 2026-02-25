package logs

import (
	"context"
	"fmt"

	"github.com/matmerr/kubectl-vmss/pkg/vmss"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

type logsOptions struct {
	namespace string
	node      string
	tail      int
	previous  bool
	pod       string

	runner  vmss.Runner
	streams genericclioptions.IOStreams
}

// NewCmdLogs returns a cobra command for "kubectl vmss logs".
func NewCmdLogs(streams genericclioptions.IOStreams) *cobra.Command {
	o := &logsOptions{
		namespace: "kube-system",
		streams:   streams,
	}

	cmd := &cobra.Command{
		Use:   "logs <pod>",
		Short: "Get container logs from the node via crictl",
		Long:  "Get container logs by resolving the pod's node and using crictl via VMSS run-command.",
		Example: `  # Get logs from a cilium pod
  kubectl vmss logs cilium-6jnvz

  # Get last 50 lines only
  kubectl vmss logs coredns-abc --tail 50

  # Get logs from previous container instance
  kubectl vmss logs cilium-6jnvz --previous`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				o.pod = args[0]
			}
			if o.pod == "" && o.node == "" {
				return fmt.Errorf("specify a pod name or --node")
			}
			if o.runner == nil {
				o.runner = vmss.NewDefaultRunner()
			}
			return o.Run(cmd.Context())
		},
	}

	cmd.Flags().StringVarP(&o.namespace, "namespace", "n", o.namespace, "Namespace for pod lookup")
	cmd.Flags().StringVar(&o.node, "node", "", "Target a node directly instead of a pod")
	cmd.Flags().IntVar(&o.tail, "tail", 0, "Number of log lines to show (0 = all)")
	cmd.Flags().BoolVar(&o.previous, "previous", false, "Show logs from previous container instance")

	return cmd
}

// Run executes the logs command.
func (o *logsOptions) Run(ctx context.Context) error {
	node := o.node
	container := ""

	if o.pod != "" {
		var err error
		container, err = o.runner.GetContainerName(ctx, o.namespace, o.pod)
		if err != nil {
			return err
		}
		if node == "" {
			node, err = o.runner.ResolveNodeFromPod(ctx, o.namespace, o.pod)
			if err != nil {
				return err
			}
		}
	}

	info, err := o.runner.ResolveVMSS(ctx, node)
	if err != nil {
		return err
	}

	script := buildLogsScript(container, o.tail, o.previous)

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

func buildLogsScript(container string, tail int, previous bool) string {
	filter := ""
	if container != "" {
		filter = fmt.Sprintf("--name %s", container)
	}

	tailFlag := ""
	if tail > 0 {
		tailFlag = fmt.Sprintf(" --tail=%d", tail)
	}

	if previous {
		return fmt.Sprintf(
			`RUNNING=$(crictl ps %s -q | head -1); ALL=$(crictl ps -a %s -q); if [ -n "$RUNNING" ]; then CID=$(echo "$ALL" | grep -v "$RUNNING" | head -1); else CID=$(echo "$ALL" | head -1); fi; if [ -z "$CID" ]; then echo 'No previous container found for %s' >&2; exit 1; fi; crictl logs%s $CID`,
			filter, filter, container, tailFlag,
		)
	}
	return fmt.Sprintf(
		`CID=$(crictl ps %s -q | head -1); if [ -z "$CID" ]; then CID=$(crictl ps -a %s -q | head -1); fi; if [ -z "$CID" ]; then echo 'No container found for %s' >&2; exit 1; fi; crictl logs%s $CID`,
		filter, filter, container, tailFlag,
	)
}
