package cmd

import (
	"fmt"

	"github.com/matmerr/kubectl-vmss/pkg/cmd/acn"
	"github.com/matmerr/kubectl-vmss/pkg/cmd/cilium"
	"github.com/matmerr/kubectl-vmss/pkg/cmd/exec"
	"github.com/matmerr/kubectl-vmss/pkg/cmd/get"
	"github.com/matmerr/kubectl-vmss/pkg/cmd/logs"
	"github.com/matmerr/kubectl-vmss/pkg/cmd/run"
	"github.com/matmerr/kubectl-vmss/pkg/version"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// NewCmdVMSS creates the root cobra command for kubectl-vmss.
func NewCmdVMSS(streams genericclioptions.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:          "kubectl-vmss",
		Short:        "Run commands on AKS nodes via Azure VMSS run-command",
		Long:         "kubectl-vmss is a kubectl plugin that resolves node, VMSS, instance ID, resource group, and subscription automatically from pod or node names, then runs commands on AKS nodes via az vmss run-command.",
		SilenceUsage: true,
	}

	cmd.AddCommand(logs.NewCmdLogs(streams))
	cmd.AddCommand(exec.NewCmdExec(streams))
	cmd.AddCommand(run.NewCmdRun(streams))
	cmd.AddCommand(get.NewCmdGet(streams))
	cmd.AddCommand(acn.NewCmdACN(streams))
	cmd.AddCommand(cilium.NewCmdCilium(streams))
	cmd.AddCommand(newCmdVersion(streams))

	return cmd
}

func newCmdVersion(streams genericclioptions.IOStreams) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(streams.Out, "kubectl-vmss %s (commit: %s, built: %s)\n",
				version.Version, version.GitCommit, version.BuildDate)
			return nil
		},
	}
}
