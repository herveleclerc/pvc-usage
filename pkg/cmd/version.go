package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/yaml"

	"github.com/herveleclerc/pvc-usage/pkg/version"
)

// VersionOptions holds options for the version command.
type VersionOptions struct {
	genericclioptions.IOStreams
	output string
}

// NewCmdVersion creates the version subcommand.
func NewCmdVersion(streams genericclioptions.IOStreams) *cobra.Command {
	o := &VersionOptions{
		IOStreams: streams,
	}

	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print plugin version and build metadata",
		RunE: func(cmd *cobra.Command, args []string) error {
			return o.Run()
		},
	}

	cmd.Flags().StringVarP(&o.output, "output", "o", "", "Output format: 'json' or 'yaml'")

	return cmd
}

// Run executes the version command.
func (o *VersionOptions) Run() error {
	info := version.Get()

	switch strings.ToLower(o.output) {
	case "json":
		data, err := json.MarshalIndent(info, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(o.Out, string(data))
	case "yaml":
		data, err := yaml.Marshal(info)
		if err != nil {
			return err
		}
		fmt.Fprint(o.Out, string(data))
	default:
		fmt.Fprintln(o.Out, info.String())
	}

	return nil
}
