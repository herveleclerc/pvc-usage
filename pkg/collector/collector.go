package collector

import (
	"context"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/herveleclerc/pvc-usage/pkg/types"
)

// Collector handles fetching and processing PVC metrics.
type Collector struct {
	client kubernetes.Interface
	nowFn  func() time.Time
}

// New creates a new Collector.
func New(client kubernetes.Interface) *Collector {
	return &Collector{
		client: client,
		nowFn:  time.Now,
	}
}

// SetNowFunc overrides current time (useful for deterministic testing).
func (c *Collector) SetNowFunc(fn func() time.Time) {
	c.nowFn = fn
}

// Result contains the list of PVC usage info and FinOps summary.
type Result struct {
	Items   []types.PVCUsageInfo `json:"items"`
	Summary types.SummaryFinOps  `json:"summary"`
}

// Collect gathers and evaluates PVC usage for the given namespace (or all if empty).
func (c *Collector) Collect(ctx context.Context, namespace string, opts types.FilterOptions) (*Result, error) {
	pvcList, err := c.client.CoreV1().PersistentVolumeClaims(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	podMap := make(map[string][]string)
	if opts.ShowPods {
		pods, err := c.client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
		if err == nil {
			for _, pod := range pods.Items {
				// Completed pods do not actively use PVC in K8s 1.37 semantics
				if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
					continue
				}
				for _, vol := range pod.Spec.Volumes {
					if vol.PersistentVolumeClaim != nil {
						key := pod.Namespace + "/" + vol.PersistentVolumeClaim.ClaimName
						podMap[key] = append(podMap[key], pod.Name)
					}
				}
			}
		}
	}

	now := c.nowFn()
	var allItems []types.PVCUsageInfo
	var filteredItems []types.PVCUsageInfo

	summary := types.SummaryFinOps{}

	for _, pvc := range pvcList.Items {
		info := c.evaluatePVC(pvc, now)
		key := pvc.Namespace + "/" + pvc.Name
		if pods, found := podMap[key]; found {
			info.ReferencingPods = pods
		}

		allItems = append(allItems, info)

		// Aggregate summary counts across all PVCs inspected
		summary.TotalPVCs++
		summary.TotalCapacityBytes += info.CapacityBytes
		if info.HasUnusedCondition {
			if info.IsUnused {
				summary.UnusedPVCs++
				summary.UnusedCapacityBytes += info.CapacityBytes
			} else {
				summary.InUsePVCs++
			}
		} else {
			summary.MissingFeaturePVCs++
		}

		// Apply filters
		if opts.UnusedOnly && !info.IsUnused {
			continue
		}
		if opts.InUseOnly && info.IsUnused {
			continue
		}
		if opts.MinUnusedAge > 0 {
			if !info.IsUnused || info.UnusedDuration < opts.MinUnusedAge {
				continue
			}
		}
		if opts.StorageClass != "" && !strings.EqualFold(info.StorageClass, opts.StorageClass) {
			continue
		}

		filteredItems = append(filteredItems, info)
	}

	summary.TotalCapacityHuman = types.FormatBytes(summary.TotalCapacityBytes)
	summary.UnusedCapacityHuman = types.FormatBytes(summary.UnusedCapacityBytes)

	// Sort items
	sortItems(filteredItems, opts.SortBy)

	return &Result{
		Items:   filteredItems,
		Summary: summary,
	}, nil
}

func (c *Collector) evaluatePVC(pvc corev1.PersistentVolumeClaim, now time.Time) types.PVCUsageInfo {
	info := types.PVCUsageInfo{
		Namespace:         pvc.Namespace,
		Name:              pvc.Name,
		Phase:             pvc.Status.Phase,
		CreationTimestamp: pvc.CreationTimestamp,
		VolumeName:        pvc.Spec.VolumeName,
		AgeHuman:          FormatDuration(now.Sub(pvc.CreationTimestamp.Time)),
	}

	// Extract access modes
	for _, am := range pvc.Spec.AccessModes {
		info.AccessModes = append(info.AccessModes, string(am))
	}

	// Extract StorageClass
	if pvc.Spec.StorageClassName != nil && *pvc.Spec.StorageClassName != "" {
		info.StorageClass = *pvc.Spec.StorageClassName
	} else if sc, ok := pvc.Annotations["volume.beta.kubernetes.io/storage-class"]; ok {
		info.StorageClass = sc
	} else {
		info.StorageClass = "<none>"
	}

	// Extract capacity (status preferred, fallback to spec requests)
	if storageQty, ok := pvc.Status.Capacity[corev1.ResourceStorage]; ok && !storageQty.IsZero() {
		info.Capacity = storageQty.String()
		info.CapacityBytes = storageQty.Value()
	} else if storageQty, ok := pvc.Spec.Resources.Requests[corev1.ResourceStorage]; ok && !storageQty.IsZero() {
		info.Capacity = storageQty.String()
		info.CapacityBytes = storageQty.Value()
	} else {
		info.Capacity = "unknown"
		info.CapacityBytes = 0
	}

	// Look for Unused condition (K8s 1.37 / KEP-5541)
	var unusedCondition *corev1.PersistentVolumeClaimCondition
	for i := range pvc.Status.Conditions {
		cond := &pvc.Status.Conditions[i]
		if cond.Type == types.ConditionTypeUnused {
			unusedCondition = cond
			break
		}
	}

	if unusedCondition == nil {
		info.HasUnusedCondition = false
		info.UsageStatus = types.StatusMissing
		info.IsUnused = false
		info.UnusedDurationStr = "N/A (K8s < 1.37)"
		return info
	}

	info.HasUnusedCondition = true
	info.Reason = unusedCondition.Reason
	info.Message = unusedCondition.Message

	switch unusedCondition.Status {
	case corev1.ConditionTrue:
		info.IsUnused = true
		info.UsageStatus = types.StatusUnused
		info.UnusedSince = &unusedCondition.LastTransitionTime
		duration := now.Sub(unusedCondition.LastTransitionTime.Time)
		if duration < 0 {
			duration = 0
		}
		info.UnusedDuration = duration
		info.UnusedDurationStr = FormatDuration(duration)

	case corev1.ConditionFalse:
		info.IsUnused = false
		info.UsageStatus = types.StatusInUse
		info.UnusedDurationStr = "Active"

	default:
		info.IsUnused = false
		info.UsageStatus = types.StatusUnknown
		info.UnusedDurationStr = string(unusedCondition.Status)
	}

	return info
}

func sortItems(items []types.PVCUsageInfo, sortBy string) {
	switch strings.ToLower(sortBy) {
	case "unused", "unused-since", "duration":
		sort.Slice(items, func(i, j int) bool {
			return items[i].UnusedDuration > items[j].UnusedDuration
		})
	case "size", "capacity":
		sort.Slice(items, func(i, j int) bool {
			return items[i].CapacityBytes > items[j].CapacityBytes
		})
	case "age", "created":
		sort.Slice(items, func(i, j int) bool {
			return items[i].CreationTimestamp.Time.Before(items[j].CreationTimestamp.Time)
		})
	case "name":
		sort.Slice(items, func(i, j int) bool {
			return items[i].Name < items[j].Name
		})
	case "namespace":
		sort.Slice(items, func(i, j int) bool {
			if items[i].Namespace == items[j].Namespace {
				return items[i].Name < items[j].Name
			}
			return items[i].Namespace < items[j].Namespace
		})
	default:
		// Default: sort by namespace, then by name
		sort.Slice(items, func(i, j int) bool {
			if items[i].Namespace == items[j].Namespace {
				return items[i].Name < items[j].Name
			}
			return items[i].Namespace < items[j].Namespace
		})
	}
}
