package collector

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var dayRegex = regexp.MustCompile(`^(\d+)([dD])$`)

// ParseDuration parses a duration string that can include 'd' or 'D' for days
// in addition to standard Go time.ParseDuration units (e.g. "30d", "7d", "24h", "90m").
func ParseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}

	matches := dayRegex.FindStringSubmatch(s)
	if len(matches) == 3 {
		days, err := strconv.Atoi(matches[1])
		if err != nil {
			return 0, fmt.Errorf("invalid day count: %w", err)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}

	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid duration format %q (examples: '30d', '7d', '24h', '2h30m'): %w", s, err)
	}
	return d, nil
}

// FormatDuration formats a duration into a compact human-readable representation,
// e.g., "14d 6h", "3h 12m", "45s".
func FormatDuration(d time.Duration) string {
	if d < 0 {
		return "0s"
	}

	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if days > 0 {
		if hours > 0 {
			return fmt.Sprintf("%dd %dh", days, hours)
		}
		return fmt.Sprintf("%dd", days)
	}

	if hours > 0 {
		if minutes > 0 {
			return fmt.Sprintf("%dh %dm", hours, minutes)
		}
		return fmt.Sprintf("%dh", hours)
	}

	if minutes > 0 {
		return fmt.Sprintf("%dm", minutes)
	}

	return fmt.Sprintf("%ds", seconds)
}
