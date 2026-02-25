package main

import (
	"os"

	"github.com/matmerr/kubectl-vmss/pkg/cmd"
	"github.com/spf13/pflag"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func main() {
	flags := pflag.NewFlagSet("kubectl-vmss", pflag.ExitOnError)
	pflag.CommandLine = flags

	streams := genericclioptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	root := cmd.NewCmdVMSS(streams)
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
