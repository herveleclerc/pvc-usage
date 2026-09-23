package versioncheck

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/discovery"
)

// CompatibilityStatus represents the cluster support level for KEP-5541.
type CompatibilityStatus string

const (
	StatusFullySupported   CompatibilityStatus = "FullySupported"   // K8s >= 1.37 (Beta, enabled by default)
	StatusAlphaSupported   CompatibilityStatus = "AlphaSupported"   // K8s == 1.36 (Alpha, requires feature gate)
	StatusUnsupported      CompatibilityStatus = "Unsupported"      // K8s < 1.36 (Feature does not exist)
	StatusUnknown          CompatibilityStatus = "Unknown"
)

// ClusterCompatibility holds version details and support assessment.
type ClusterCompatibility struct {
	GitVersion string              `json:"gitVersion"`
	Major      int                 `json:"major"`
	Minor      int                 `json:"minor"`
	Status     CompatibilityStatus `json:"status"`
	Warning    string              `json:"warning,omitempty"`
}

var minorRegex = regexp.MustCompile(`^(\d+)`)

// CheckServerVersion inspects the Kubernetes server version via Discovery client.
func CheckServerVersion(discoveryClient discovery.ServerVersionInterface) (*ClusterCompatibility, error) {
	sv, err := discoveryClient.ServerVersion()
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve Kubernetes server version: %w", err)
	}

	return EvaluateVersion(sv), nil
}

// EvaluateVersion parses and classifies a ServerVersion struct.
func EvaluateVersion(sv *version.Info) *ClusterCompatibility {
	comp := &ClusterCompatibility{
		GitVersion: sv.GitVersion,
		Status:     StatusUnknown,
	}

	major, err := strconv.Atoi(strings.TrimSpace(sv.Major))
	if err != nil {
		major = 1
	}
	comp.Major = major

	minorStr := strings.TrimSpace(sv.Minor)
	matches := minorRegex.FindStringSubmatch(minorStr)
	if len(matches) >= 2 {
		if m, err := strconv.Atoi(matches[1]); err == nil {
			comp.Minor = m
		}
	}

	if comp.Major == 1 {
		switch {
		case comp.Minor >= 37:
			comp.Status = StatusFullySupported
		case comp.Minor == 36:
			comp.Status = StatusAlphaSupported
			comp.Warning = fmt.Sprintf("Kubernetes server is running version %s (v1.36). Feature 'PersistentVolumeClaimUnusedSinceTime' is Alpha and requires the feature gate to be explicitly enabled on kube-controller-manager.", sv.GitVersion)
		default:
			comp.Status = StatusUnsupported
			comp.Warning = fmt.Sprintf("Kubernetes server is running version %s (v1.%d). The PVC 'Unused' condition requires Kubernetes >= 1.37 (or 1.36 with feature gate enabled). All PVCs will report status 'FeatureNotPresent'.", sv.GitVersion, comp.Minor)
		}
	} else if comp.Major > 1 {
		comp.Status = StatusFullySupported
	} else {
		comp.Status = StatusUnsupported
		comp.Warning = fmt.Sprintf("Unsupported Kubernetes version: %s", sv.GitVersion)
	}

	return comp
}
