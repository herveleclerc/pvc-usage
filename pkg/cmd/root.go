package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"

	"github.com/herveleclerc/pvc-usage/pkg/collector"
	"github.com/herveleclerc/pvc-usage/pkg/printer"
	"github.com/herveleclerc/pvc-usage/pkg/types"
	"github.com/herveleclerc/pvc-usage/pkg/versioncheck"
)

// RootOptions holds configuration and flags for the root command.
type RootOptions struct {
	configFlags *genericclioptions.ConfigFlags
	genericclioptions.IOStreams

	allNamespaces    bool
	unusedOnly       bool
	inUseOnly        bool
	minAgeStr        string
	storageClass     string
	sortBy           string
	showPods         bool
	output           string
	showSummary      bool
	noHeaders        bool
	skipVersionCheck bool
}

// NewRootOptions returns a default RootOptions.
func NewRootOptions(streams genericclioptions.IOStreams) *RootOptions {
	return &RootOptions{
		configFlags: genericclioptions.NewConfigFlags(true),
		IOStreams:   streams,
		showSummary: true,
		sortBy:      "namespace",
		output:      "table",
	}
}

// NewCmdRoot creates the root command for kubectl-pvc-usage.
func NewCmdRoot(streams genericclioptions.IOStreams) *cobra.Command {
	o := NewRootOptions(streams)

	cmd := &cobra.Command{
		Use:   "pvc-usage",
		Short: "Inspect PersistentVolumeClaim usage and idle duration using Kubernetes 1.37 Unused condition",
		Long: `kubectl-pvc-usage is a kubectl plugin designed to detect unused PersistentVolumeClaims.
It natively leverages the Kubernetes 1.37 Beta feature 'PersistentVolumeClaimUnusedSinceTime' (KEP-5541)
to identify orphaned volumes, calculate idle time, and help reclaim storage capacity.`,
		Example: `  # List all PVCs in current namespace with usage status
  kubectl pvc-usage

  # List unused PVCs across all namespaces
  kubectl pvc-usage -A --unused-only

  # List PVCs idle for more than 30 days
  kubectl pvc-usage -A --min-age=30d

  # Show detailed view including pods referencing PVCs
  kubectl pvc-usage -A -o wide --show-pods

  # Output in JSON format sorted by idle duration
  kubectl pvc-usage -A --unused-only --sort-by=unused -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return o.Run(cmd.Context())
		},
	}

	// Add standard kubectl flags (--kubeconfig, --context, -n/--namespace, -A/--all-namespaces, etc.)
	o.configFlags.AddFlags(cmd.Flags())

	// Add plugin-specific flags
	cmd.Flags().BoolVarP(&o.allNamespaces, "all-namespaces", "A", o.allNamespaces, "If present, list the requested object(s) across all namespaces. Namespace in current context is ignored even if specified with --namespace.")
	cmd.Flags().BoolVarP(&o.unusedOnly, "unused-only", "u", o.unusedOnly, "Only display PVCs currently marked as unused")
	cmd.Flags().BoolVar(&o.inUseOnly, "in-use-only", o.inUseOnly, "Only display PVCs currently referenced by pods")
	cmd.Flags().StringVar(&o.minAgeStr, "min-age", o.minAgeStr, "Filter PVCs unused for at least this duration (e.g. '30d', '7d', '24h')")
	cmd.Flags().StringVar(&o.storageClass, "storage-class", o.storageClass, "Filter by StorageClass name")
	cmd.Flags().StringVar(&o.sortBy, "sort-by", o.sortBy, "Sort results by: 'namespace', 'name', 'unused', 'size', 'age'")
	cmd.Flags().BoolVar(&o.showPods, "show-pods", o.showPods, "Cross-reference pods in namespaces to list referencing pods")
	cmd.Flags().StringVarP(&o.output, "output", "o", o.output, "Output format: 'table', 'wide', 'json', 'yaml'")
	cmd.Flags().BoolVar(&o.showSummary, "summary", o.showSummary, "Display FinOps storage summary at end of table output")
	cmd.Flags().BoolVar(&o.noHeaders, "no-headers", o.noHeaders, "Do not print table header")
	cmd.Flags().BoolVar(&o.skipVersionCheck, "skip-version-check", false, "Skip Kubernetes server version compatibility check")

	// Subcommands
	cmd.AddCommand(NewCmdVersion(streams))

	return cmd
}

// Run executes the collector and outputs formatted results.
func (o *RootOptions) Run(ctx context.Context) error {
	minAge, err := collector.ParseDuration(o.minAgeStr)
	if err != nil {
		return err
	}

	restConfig, err := o.configFlags.ToRESTConfig()
	if err != nil {
		return fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	client, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	// Verify Kubernetes cluster version compatibility with KEP-5541 (Beta in v1.37)
	if !o.skipVersionCheck {
		comp, err := versioncheck.CheckServerVersion(client.Discovery())
		if err == nil && comp.Warning != "" {
			fmt.Fprintf(o.ErrOut, "Warning: %s\n\n", comp.Warning)
		}
	}

	namespace := ""
	if o.allNamespaces {
		namespace = ""
	} else if o.configFlags.Namespace != nil && *o.configFlags.Namespace != "" {
		namespace = *o.configFlags.Namespace
	} else {
		ns, _, err := o.configFlags.ToRawKubeConfigLoader().Namespace()
		if err == nil {
			namespace = ns
		}
	}

	filterOpts := types.FilterOptions{
		UnusedOnly:   o.unusedOnly,
		InUseOnly:    o.inUseOnly,
		MinUnusedAge: minAge,
		StorageClass: o.storageClass,
		SortBy:       o.sortBy,
		ShowPods:     o.showPods,
	}

	c := collector.New(client)
	res, err := c.Collect(ctx, namespace, filterOpts)
	if err != nil {
		return fmt.Errorf("error collecting PVC metrics: %w", err)
	}

	printOpts := printer.PrintOptions{
		OutputFormat: o.output,
		ShowSummary:  o.showSummary,
		NoHeaders:    o.noHeaders,
	}

	return printer.Print(o.Out, res, printOpts)
}
