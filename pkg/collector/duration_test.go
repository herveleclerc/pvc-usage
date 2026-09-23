package collector

import (
	"testing"
	"time"
)

func TestParseDuration(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
		wantErr  bool
	}{
		{"", 0, false},
		{"30d", 30 * 24 * time.Hour, false},
		{"7D", 7 * 24 * time.Hour, false},
		{"24h", 24 * time.Hour, false},
		{"90m", 90 * time.Minute, false},
		{"1h30m", 90 * time.Minute, false},
		{"invalid", 0, true},
	}

	for _, tc := range tests {
		got, err := ParseDuration(tc.input)
		if (err != nil) != tc.wantErr {
			t.Errorf("ParseDuration(%q) unexpected error: %v", tc.input, err)
			continue
		}
		if got != tc.expected {
			t.Errorf("ParseDuration(%q) = %v, expected %v", tc.input, got, tc.expected)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		duration time.Duration
		expected string
	}{
		{-1 * time.Second, "0s"},
		{0, "0s"},
		{45 * time.Second, "45s"},
		{15 * time.Minute, "15m"},
		{3 * time.Hour, "3h"},
		{3*time.Hour + 25*time.Minute, "3h 25m"},
		{4 * 24 * time.Hour, "4d"},
		{4*24*time.Hour + 6*time.Hour, "4d 6h"},
	}

	for _, tc := range tests {
		got := FormatDuration(tc.duration)
		if got != tc.expected {
			t.Errorf("FormatDuration(%v) = %q, expected %q", tc.duration, got, tc.expected)
		}
	}
}
