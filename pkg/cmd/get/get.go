package get

import (
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// NewCmdGet returns the parent "get" command with subcommands pods and netns.
func NewCmdGet(streams genericclioptions.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Display node-level resources via VMSS run-command",
		Long:  "Retrieve node-level information (pods, network namespaces) by running commands on AKS nodes via VMSS run-command.",
	}

	cmd.AddCommand(NewCmdGetPods(streams))
	cmd.AddCommand(NewCmdGetNetns(streams))

	return cmd
}
