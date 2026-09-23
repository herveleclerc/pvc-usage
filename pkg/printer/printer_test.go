package printer

import (
	"bytes"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/herveleclerc/pvc-usage/pkg/collector"
	"github.com/herveleclerc/pvc-usage/pkg/types"
)

func TestPrintTable(t *testing.T) {
	now := time.Now()
	res := &collector.Result{
		Items: []types.PVCUsageInfo{
			{
				Namespace:         "prod",
				Name:              "data-db-0",
				Phase:             corev1.ClaimBound,
				Capacity:          "100Gi",
				CapacityBytes:     107374182400,
				StorageClass:      "gp3",
				VolumeName:        "vol-0123",
				AccessModes:       []string{"ReadWriteOnce"},
				CreationTimestamp: metav1.NewTime(now.Add(-40 * 24 * time.Hour)),
				UsageStatus:       types.StatusUnused,
				IsUnused:          true,
				UnusedDurationStr: "28d 4h",
				AgeHuman:          "40d",
				Reason:            types.ReasonNoPodsUsingPVC,
			},
		},
		Summary: types.SummaryFinOps{
			TotalPVCs:           1,
			UnusedPVCs:          1,
			TotalCapacityBytes:  107374182400,
			UnusedCapacityBytes: 107374182400,
			TotalCapacityHuman:  "100Gi",
			UnusedCapacityHuman: "100Gi",
		},
	}

	t.Run("Standard Table", func(t *testing.T) {
		var buf bytes.Buffer
		err := Print(&buf, res, PrintOptions{OutputFormat: "table", ShowSummary: true})
		if err != nil {
			t.Fatalf("Print error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "NAMESPACE") || !strings.Contains(out, "data-db-0") {
			t.Errorf("expected table headers and item name in output: %s", out)
		}
		if !strings.Contains(out, "28d 4h") {
			t.Errorf("expected unused duration in output: %s", out)
		}
		if !strings.Contains(out, "Storage Usage Summary (FinOps)") {
			t.Errorf("expected summary section: %s", out)
		}
	})

	t.Run("Wide Table", func(t *testing.T) {
		var buf bytes.Buffer
		err := Print(&buf, res, PrintOptions{OutputFormat: "wide"})
		if err != nil {
			t.Fatalf("Print error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "VOLUME") || !strings.Contains(out, "ACCESSMODES") {
			t.Errorf("expected wide headers in output: %s", out)
		}
		if !strings.Contains(out, "vol-0123") {
			t.Errorf("expected volume name: %s", out)
		}
	})

	t.Run("JSON output", func(t *testing.T) {
		var buf bytes.Buffer
		err := Print(&buf, res, PrintOptions{OutputFormat: "json"})
		if err != nil {
			t.Fatalf("Print error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, `"name": "data-db-0"`) {
			t.Errorf("expected json structure: %s", out)
		}
	})

	t.Run("YAML output", func(t *testing.T) {
		var buf bytes.Buffer
		err := Print(&buf, res, PrintOptions{OutputFormat: "yaml"})
		if err != nil {
			t.Fatalf("Print error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "name: data-db-0") {
			t.Errorf("expected yaml structure: %s", out)
		}
	})
}
