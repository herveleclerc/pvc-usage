package main

import (
	"os"

	"github.com/spf13/pflag"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/herveleclerc/pvc-usage/pkg/cmd"
)

func main() {
	flags := pflag.NewFlagSet("kubectl-pvc-usage", pflag.ExitOnError)
	pflag.CommandLine = flags

	streams := genericclioptions.IOStreams{
		In:     os.Stdin,
		Out:    os.Stdout,
		ErrOut: os.Stderr,
	}

	rootCmd := cmd.NewCmdRoot(streams)
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
