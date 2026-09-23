package types

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ConditionTypeUnused is the condition type introduced in Kubernetes 1.36 (Alpha) and 1.37 (Beta)
// via the PersistentVolumeClaimUnusedSinceTime feature gate (KEP-5541).
const ConditionTypeUnused corev1.PersistentVolumeClaimConditionType = "Unused"

// UnusedReason constants defined in Kubernetes 1.37.
const (
	ReasonNoPodsUsingPVC = "NoPodsUsingPVC"
	ReasonPodUsingPVC    = "PodUsingPVC"
)

// PVCUsageStatus represents the interpreted status of the PVC usage.
type PVCUsageStatus string

const (
	StatusUnused  PVCUsageStatus = "Unused"
	StatusInUse   PVCUsageStatus = "InUse"
	StatusUnknown PVCUsageStatus = "Unknown"
	StatusMissing PVCUsageStatus = "FeatureNotPresent"
)

// PVCUsageInfo holds the processed usage metrics for a single PVC.
type PVCUsageInfo struct {
	Namespace          string                            `json:"namespace"`
	Name               string                            `json:"name"`
	Phase              corev1.PersistentVolumeClaimPhase `json:"phase"`
	Capacity           string                            `json:"capacity"`
	CapacityBytes      int64                             `json:"capacityBytes"`
	StorageClass       string                            `json:"storageClass"`
	VolumeName         string                            `json:"volumeName"`
	AccessModes        []string                          `json:"accessModes"`
	CreationTimestamp  metav1.Time                       `json:"creationTimestamp"`
	UsageStatus        PVCUsageStatus                    `json:"usageStatus"`
	IsUnused           bool                              `json:"isUnused"`
	HasUnusedCondition bool                              `json:"hasUnusedCondition"`
	UnusedSince        *metav1.Time                      `json:"unusedSince,omitempty"`
	UnusedDuration     time.Duration                     `json:"unusedDuration"`
	UnusedDurationStr  string                            `json:"unusedDurationHuman"`
	AgeHuman           string                            `json:"ageHuman"`
	Reason             string                            `json:"reason,omitempty"`
	Message            string                            `json:"message,omitempty"`
	ReferencingPods    []string                          `json:"referencingPods,omitempty"`
}

// SummaryFinOps provides aggregate statistics about examined PVCs.
type SummaryFinOps struct {
	TotalPVCs           int    `json:"totalPVCs"`
	UnusedPVCs          int    `json:"unusedPVCs"`
	InUsePVCs           int    `json:"inUsePVCs"`
	MissingFeaturePVCs  int    `json:"missingFeaturePVCs"`
	TotalCapacityBytes  int64  `json:"totalCapacityBytes"`
	UnusedCapacityBytes int64  `json:"unusedCapacityBytes"`
	TotalCapacityHuman  string `json:"totalCapacityHuman"`
	UnusedCapacityHuman string `json:"unusedCapacityHuman"`
}

// FilterOptions defines filtering and sorting criteria for collector.
type FilterOptions struct {
	UnusedOnly   bool
	InUseOnly    bool
	MinUnusedAge time.Duration
	StorageClass string
	SortBy       string
	ShowPods     bool
}

// FormatBytes converts a byte count into a human-readable string (e.g. 50Gi, 2.5Ti).
func FormatBytes(bytes int64) string {
	q := resource.NewQuantity(bytes, resource.BinarySI)
	return q.String()
}
