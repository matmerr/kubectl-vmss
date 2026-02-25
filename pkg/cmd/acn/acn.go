package acn

import (
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// NewCmdACN returns the parent "acn" command with subcommands logs and state.
func NewCmdACN(streams genericclioptions.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "acn",
		Aliases: []string{"azcni", "cni"},
		Short:   "Inspect Azure CNI / CNS on a node via VMSS run-command",
		Long:    "Retrieve Azure CNI / Azure CNS logs and state files from AKS nodes via VMSS run-command.",
	}

	cmd.AddCommand(NewCmdACNLogs(streams))
	cmd.AddCommand(NewCmdACNState(streams))

	return cmd
}
