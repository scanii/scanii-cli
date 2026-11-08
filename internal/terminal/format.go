package terminal

import (
	"fmt"
	"strings"
	"time"
)

// FormatBytes formats a byte count into a human-readable string (e.g., "1.2 MB").
func FormatBytes(b uint64) string {
	const unit = 1000
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	val := float64(b) / float64(div)
	units := []string{"KB", "MB", "GB", "TB", "PB", "EB"}
	s := fmt.Sprintf("%.1f", val)
	s = strings.TrimSuffix(s, ".0")
	return s + " " + units[exp]
}

// FormatNumber formats an integer with comma separators (e.g., 1500 → "1,500").
func FormatNumber(n int64) string {
	if n < 0 {
		return "-" + FormatNumber(-n)
	}
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var result []byte
	for i, c := range s {
		remaining := len(s) - i
		if remaining%3 == 0 && i != 0 {
			result = append(result, ',')
		}
		result = append(result, byte(c)) // #nosec G115 -- s is %d output: ASCII digits and '-'
	}
	return string(result)
}

// FormatDuration formats a duration for human display with at most 1 decimal place.
// Examples: "245 ms", "2.5 s", "1.2 min", "3 h", "2 d"
func FormatDuration(d time.Duration) string {
	switch {
	case d < time.Millisecond:
		us := float64(d.Microseconds())
		return formatUnit(us, "μs")
	case d < time.Second:
		ms := float64(d.Milliseconds())
		return formatUnit(ms, "ms")
	case d < time.Minute:
		s := d.Seconds()
		return formatUnit(s, "s")
	case d < time.Hour:
		m := d.Minutes()
		return formatUnit(m, "min")
	case d < 24*time.Hour:
		h := d.Hours()
		return formatUnit(h, "h")
	default:
		days := d.Hours() / 24
		return formatUnit(days, "d")
	}
}

func formatUnit(value float64, unit string) string {
	s := fmt.Sprintf("%.1f", value)
	// strip trailing ".0" for clean whole numbers
	s = strings.TrimSuffix(s, ".0")
	return s + " " + unit
}

// FormatTime parses an ISO8601/RFC3339 time string and returns it formatted
// in RFC1123 in the local timezone. If parsing fails, the original string is returned.
func FormatTime(s string) string {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		// try without nanoseconds
		t, err = time.Parse(time.RFC3339, s)
		if err != nil {
			return s
		}
	}
	return t.Local().Format(time.RFC1123)
}
