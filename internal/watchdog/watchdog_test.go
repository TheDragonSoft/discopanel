package watchdog

import (
	"testing"
	"time"
)

func TestNextBackoff(t *testing.T) {
	base30 := 30 * time.Second

	tests := []struct {
		name        string
		baseSecs    int
		consecutive int
		want        time.Duration
	}{
		{"first crash uses base", 30, 1, base30},
		{"second crash doubles", 30, 2, 60 * time.Second},
		{"third crash doubles again", 30, 3, 120 * time.Second},
		{"base of 10", 10, 2, 20 * time.Second},
		{"capped at 10x base", 30, 5, 300 * time.Second},
		{"capped at 10x base even for huge counts", 30, 50, 300 * time.Second},
		{"cap reached exactly", 30, 6, 300 * time.Second},
		{"zero base falls back to 30s", 0, 1, base30},
		{"negative base falls back to 30s", -5, 2, 60 * time.Second},
		{"one second base caps at 10s", 1, 20, 10 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nextBackoff(tt.baseSecs, tt.consecutive)
			if got != tt.want {
				t.Errorf("nextBackoff(%d, %d) = %v, want %v", tt.baseSecs, tt.consecutive, got, tt.want)
			}
		})
	}
}
