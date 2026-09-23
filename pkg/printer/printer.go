package printer

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"sigs.k8s.io/yaml"

	"github.com/herveleclerc/pvc-usage/pkg/collector"
	"github.com/herveleclerc/pvc-usage/pkg/types"
)

// PrintOptions controls output formatting.
type PrintOptions struct {
	OutputFormat string // "table", "wide", "json", "yaml"
	ShowSummary  bool
	NoHeaders    bool
}

// Print outputs the collector Result according to the PrintOptions.
func Print(w io.Writer, res *collector.Result, opts PrintOptions) error {
	switch strings.ToLower(opts.OutputFormat) {
	case "json":
		return printJSON(w, res)
	case "yaml":
		return printYAML(w, res)
	case "wide":
		return printTable(w, res, true, opts)
	default:
		return printTable(w, res, false, opts)
	}
}

func printJSON(w io.Writer, res *collector.Result) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(res)
}

func printYAML(w io.Writer, res *collector.Result) error {
	data, err := yaml.Marshal(res)
	if err != nil {
		return fmt.Errorf("failed to marshal YAML: %w", err)
	}
	_, err = w.Write(data)
	return err
}

func printTable(w io.Writer, res *collector.Result, wide bool, opts PrintOptions) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)

	if !opts.NoHeaders {
		if wide {
			fmt.Fprintln(tw, "NAMESPACE\tNAME\tSTATUS\tUNUSED-SINCE\tCAPACITY\tSTORAGECLASS\tVOLUME\tACCESSMODES\tPODS\tAGE\tREASON")
		} else {
			fmt.Fprintln(tw, "NAMESPACE\tNAME\tSTATUS\tUNUSED-SINCE\tCAPACITY\tSTORAGECLASS\tAGE")
		}
	}

	for _, item := range res.Items {
		statusStr := string(item.UsageStatus)
		unusedStr := item.UnusedDurationStr

		if wide {
			podsStr := "<none>"
			if len(item.ReferencingPods) > 0 {
				podsStr = strings.Join(item.ReferencingPods, ",")
			}
			modesStr := strings.Join(item.AccessModes, ",")
			reasonStr := item.Reason
			if reasonStr == "" {
				reasonStr = "<none>"
			}

			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				item.Namespace,
				item.Name,
				statusStr,
				unusedStr,
				item.Capacity,
				item.StorageClass,
				item.VolumeName,
				modesStr,
				podsStr,
				item.AgeHuman,
				reasonStr,
			)
		} else {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				item.Namespace,
				item.Name,
				statusStr,
				unusedStr,
				item.Capacity,
				item.StorageClass,
				item.AgeHuman,
			)
		}
	}

	if err := tw.Flush(); err != nil {
		return err
	}

	if opts.ShowSummary {
		printSummary(w, res.Summary)
	}

	return nil
}

func printSummary(w io.Writer, s types.SummaryFinOps) {
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "--- Storage Usage Summary (FinOps) ---")
	fmt.Fprintf(w, "Total PVCs examined:   %d (Total capacity: %s)\n", s.TotalPVCs, s.TotalCapacityHuman)
	fmt.Fprintf(w, "Unused PVCs:           %d (Orphaned capacity: %s)\n", s.UnusedPVCs, s.UnusedCapacityHuman)
	fmt.Fprintf(w, "Active in-use PVCs:    %d\n", s.InUsePVCs)
	if s.MissingFeaturePVCs > 0 {
		fmt.Fprintf(w, "Missing Unused cond:   %d (Requires K8s >= 1.37 or KEP-5541 enabled)\n", s.MissingFeaturePVCs)
	}
}
