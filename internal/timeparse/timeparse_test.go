package timeparse

import (
	"testing"
	"time"
)

func TestParseSince(t *testing.T) {
	now := time.Date(2026, 4, 26, 18, 0, 0, 0, time.UTC)
	cases := []struct {
		in      string
		want    time.Time
		wantErr bool
	}{
		{"30s", now.Add(-30 * time.Second), false},
		{"15m", now.Add(-15 * time.Minute), false},
		{"6h", now.Add(-6 * time.Hour), false},
		{"1d", now.Add(-24 * time.Hour), false},
		{"2w", now.Add(-14 * 24 * time.Hour), false},
		{"2026-04-25", time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC), false},
		{"2026-04-25T10:00:00Z", time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC), false},
		{"2026-04-25T10:00:00+02:00", time.Date(2026, 4, 25, 8, 0, 0, 0, time.UTC), false},
		{"", time.Time{}, true},
		{"1.5d", time.Time{}, true},
		{"d", time.Time{}, true},
		{"1y", time.Time{}, true},
		{"-1d", time.Time{}, true},
		{"not-a-date", time.Time{}, true},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, err := ParseSince(tc.in, now)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tc.wantErr)
			}
			if !tc.wantErr && !got.Equal(tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}
