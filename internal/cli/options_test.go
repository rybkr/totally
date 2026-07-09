package cli

import (
	"testing"
	"time"
)

func TestParseTimeBoundRelativeDays(t *testing.T) {
	now := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)

	got, err := parseTimeBound("7d", now, false)
	if err != nil {
		t.Fatal(err)
	}

	want := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestParseTimeBoundDateOnlyUntilIncludesWholeDay(t *testing.T) {
	got, err := parseTimeBound("2026-07-09", time.Now(), true)
	if err != nil {
		t.Fatal(err)
	}

	start := time.Date(2026, 7, 9, 0, 0, 0, 0, time.Local)
	want := start.AddDate(0, 0, 1).Add(-time.Nanosecond)
	if !got.Equal(want) {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestGlobalOptionsRejectsInvertedTimeRange(t *testing.T) {
	_, err := globalOptions{
		since: "2026-07-10",
		until: "2026-07-09",
	}.timeRange(time.Now())
	if err == nil {
		t.Fatal("expected inverted range to fail")
	}
}
