// Package timeparse parses the --since CLI flag, which accepts either a
// relative duration (e.g. "1d", "6h", "30m", "2w") or an ISO-8601
// date/datetime. Naive dates are interpreted as UTC.
package timeparse

import (
	"fmt"
	"regexp"
	"strconv"
	"time"
)

var durationRe = regexp.MustCompile(`^(\d+)([smhdw])$`)

// ParseSince resolves the --since flag to an absolute UTC timestamp.
// `now` is injected for testability and is treated as the upper bound
// for duration-relative inputs.
func ParseSince(s string, now time.Time) (time.Time, error) {
	if s == "" {
		return time.Time{}, fmt.Errorf("--since: empty value")
	}
	if m := durationRe.FindStringSubmatch(s); m != nil {
		n, err := strconv.Atoi(m[1])
		if err != nil {
			return time.Time{}, fmt.Errorf("--since: duration value too large: %q", m[1])
		}
		var unit time.Duration
		switch m[2] {
		case "s":
			unit = time.Second
		case "m":
			unit = time.Minute
		case "h":
			unit = time.Hour
		case "d":
			unit = 24 * time.Hour
		case "w":
			unit = 7 * 24 * time.Hour
		}
		return now.Add(-time.Duration(n) * unit).UTC(), nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("--since: %q is not a duration (e.g. 1d, 6h) or an ISO-8601 date", s)
}
