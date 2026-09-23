package collector

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/herveleclerc/pvc-usage/pkg/types"
)

func TestCollector(t *testing.T) {
	now := time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)
	scFast := "fast-ssd"
	scStandard := "standard"

	pvcUnusedOld := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "pvc-unused-old",
			Namespace:         "default",
			CreationTimestamp: metav1.NewTime(now.Add(-60 * 24 * time.Hour)),
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: &scFast,
			VolumeName:       "pv-fast-01",
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("50Gi"),
				},
			},
		},
		Status: corev1.PersistentVolumeClaimStatus{
			Phase: corev1.ClaimBound,
			Capacity: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse("50Gi"),
			},
			Conditions: []corev1.PersistentVolumeClaimCondition{
				{
					Type:               types.ConditionTypeUnused,
					Status:             corev1.ConditionTrue,
					Reason:             types.ReasonNoPodsUsingPVC,
					Message:            "No pods are currently referencing this PVC",
					LastTransitionTime: metav1.NewTime(now.Add(-35 * 24 * time.Hour)), // 35 days unused
				},
			},
		},
	}

	pvcInUse := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "pvc-active",
			Namespace:         "default",
			CreationTimestamp: metav1.NewTime(now.Add(-10 * 24 * time.Hour)),
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: &scStandard,
			VolumeName:       "pv-std-02",
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("10Gi"),
				},
			},
		},
		Status: corev1.PersistentVolumeClaimStatus{
			Phase: corev1.ClaimBound,
			Capacity: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse("10Gi"),
			},
			Conditions: []corev1.PersistentVolumeClaimCondition{
				{
					Type:               types.ConditionTypeUnused,
					Status:             corev1.ConditionFalse,
					Reason:             types.ReasonPodUsingPVC,
					LastTransitionTime: metav1.NewTime(now.Add(-5 * 24 * time.Hour)),
				},
			},
		},
	}

	pvcLegacy := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "pvc-legacy-k8s",
			Namespace:         "kube-system",
			CreationTimestamp: metav1.NewTime(now.Add(-5 * 24 * time.Hour)),
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: &scStandard,
			VolumeName:       "pv-std-03",
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("5Gi"),
				},
			},
		},
		Status: corev1.PersistentVolumeClaimStatus{
			Phase: corev1.ClaimBound,
			Capacity: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse("5Gi"),
			},
			// No Conditions
		},
	}

	fakeClient := fake.NewSimpleClientset(pvcUnusedOld, pvcInUse, pvcLegacy)
	c := New(fakeClient)
	c.SetNowFunc(func() time.Time { return now })

	ctx := context.Background()

	t.Run("Collect all without filter", func(t *testing.T) {
		res, err := c.Collect(ctx, "", types.FilterOptions{})
		if err != nil {
			t.Fatalf("Collect error: %v", err)
		}
		if len(res.Items) != 3 {
			t.Fatalf("expected 3 items, got %d", len(res.Items))
		}
		if res.Summary.TotalPVCs != 3 {
			t.Errorf("expected 3 total PVCs, got %d", res.Summary.TotalPVCs)
		}
		if res.Summary.UnusedPVCs != 1 {
			t.Errorf("expected 1 unused PVC, got %d", res.Summary.UnusedPVCs)
		}
		if res.Summary.InUsePVCs != 1 {
			t.Errorf("expected 1 in-use PVC, got %d", res.Summary.InUsePVCs)
		}
		if res.Summary.MissingFeaturePVCs != 1 {
			t.Errorf("expected 1 missing feature PVC, got %d", res.Summary.MissingFeaturePVCs)
		}
	})

	t.Run("Collect with UnusedOnly filter", func(t *testing.T) {
		res, err := c.Collect(ctx, "", types.FilterOptions{UnusedOnly: true})
		if err != nil {
			t.Fatalf("Collect error: %v", err)
		}
		if len(res.Items) != 1 {
			t.Fatalf("expected 1 item, got %d", len(res.Items))
		}
		if res.Items[0].Name != "pvc-unused-old" {
			t.Errorf("expected pvc-unused-old, got %s", res.Items[0].Name)
		}
		if res.Items[0].UnusedDurationStr != "35d" {
			t.Errorf("expected 35d, got %s", res.Items[0].UnusedDurationStr)
		}
	})

	t.Run("Collect with MinUnusedAge filter", func(t *testing.T) {
		// Filter > 40 days: should return 0 items
		res, err := c.Collect(ctx, "", types.FilterOptions{MinUnusedAge: 40 * 24 * time.Hour})
		if err != nil {
			t.Fatalf("Collect error: %v", err)
		}
		if len(res.Items) != 0 {
			t.Errorf("expected 0 items, got %d", len(res.Items))
		}

		// Filter > 30 days: should return 1 item
		res, err = c.Collect(ctx, "", types.FilterOptions{MinUnusedAge: 30 * 24 * time.Hour})
		if err != nil {
			t.Fatalf("Collect error: %v", err)
		}
		if len(res.Items) != 1 {
			t.Errorf("expected 1 item, got %d", len(res.Items))
		}
	})

	t.Run("Collect with StorageClass filter", func(t *testing.T) {
		res, err := c.Collect(ctx, "", types.FilterOptions{StorageClass: "fast-ssd"})
		if err != nil {
			t.Fatalf("Collect error: %v", err)
		}
		if len(res.Items) != 1 {
			t.Fatalf("expected 1 item, got %d", len(res.Items))
		}
		if res.Items[0].Name != "pvc-unused-old" {
			t.Errorf("expected pvc-unused-old, got %s", res.Items[0].Name)
		}
	})
}
