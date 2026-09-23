package versioncheck

import (
	"testing"

	"k8s.io/apimachinery/pkg/version"
)

func TestEvaluateVersion(t *testing.T) {
	tests := []struct {
		name           string
		info           *version.Info
		expectedStatus CompatibilityStatus
		hasWarning     bool
	}{
		{
			name: "Kubernetes 1.37 GA/Beta",
			info: &version.Info{
				Major:      "1",
				Minor:      "37",
				GitVersion: "v1.37.0",
			},
			expectedStatus: StatusFullySupported,
			hasWarning:     false,
		},
		{
			name: "Kubernetes 1.37 with trailing plus",
			info: &version.Info{
				Major:      "1",
				Minor:      "37+",
				GitVersion: "v1.37.0-alpha.2",
			},
			expectedStatus: StatusFullySupported,
			hasWarning:     false,
		},
		{
			name: "Kubernetes 1.38 future version",
			info: &version.Info{
				Major:      "1",
				Minor:      "38",
				GitVersion: "v1.38.0",
			},
			expectedStatus: StatusFullySupported,
			hasWarning:     false,
		},
		{
			name: "Kubernetes 1.36 Alpha",
			info: &version.Info{
				Major:      "1",
				Minor:      "36",
				GitVersion: "v1.36.2",
			},
			expectedStatus: StatusAlphaSupported,
			hasWarning:     true,
		},
		{
			name: "Kubernetes 1.35 Unsupported",
			info: &version.Info{
				Major:      "1",
				Minor:      "35",
				GitVersion: "v1.35.4",
			},
			expectedStatus: StatusUnsupported,
			hasWarning:     true,
		},
		{
			name: "Kubernetes 1.31 Unsupported",
			info: &version.Info{
				Major:      "1",
				Minor:      "31+",
				GitVersion: "v1.31.1",
			},
			expectedStatus: StatusUnsupported,
			hasWarning:     true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := EvaluateVersion(tc.info)
			if res.Status != tc.expectedStatus {
				t.Errorf("expected status %v, got %v", tc.expectedStatus, res.Status)
			}
			if (res.Warning != "") != tc.hasWarning {
				t.Errorf("warning mismatch: got %q, expected hasWarning=%v", res.Warning, tc.hasWarning)
			}
		})
	}
}
