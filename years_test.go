package main

import (
	"testing"
	"time"
)

func TestRecentPublicationYearRange(t *testing.T) {
	start, end := recentPublicationYearRange(time.Date(2026, time.July, 18, 0, 0, 0, 0, time.UTC))
	if start != 2021 || end != 2026 {
		t.Fatalf("recentPublicationYearRange() = %d-%d, want 2021-2026", start, end)
	}
	if got := end - start + 1; got != recentPublicationYearCount {
		t.Fatalf("range covers %d years, want %d", got, recentPublicationYearCount)
	}
}
