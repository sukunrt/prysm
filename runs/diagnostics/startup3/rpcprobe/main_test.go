package main

import (
	"testing"
	"time"
)

func TestNextProbeDueSkipsMissedTicks(t *testing.T) {
	base := time.Unix(100, 0)
	interval := 500 * time.Millisecond
	for _, tc := range []struct {
		now, want time.Duration
	}{
		{100 * time.Millisecond, 500 * time.Millisecond},
		{500 * time.Millisecond, time.Second},
		{2100 * time.Millisecond, 2500 * time.Millisecond},
	} {
		got := nextProbeDue(base, base.Add(tc.now), interval)
		if want := base.Add(tc.want); !got.Equal(want) {
			t.Fatalf("now %s: got %s, want %s", tc.now, got.Sub(base), want.Sub(base))
		}
	}
}
