package main

import "testing"

func TestBuildVersion(t *testing.T) {
	originalVersion, originalCommit, originalDate := version, commit, date
	t.Cleanup(func() {
		version, commit, date = originalVersion, originalCommit, originalDate
	})

	version = "v0.4.2"
	commit = "abc1234"
	date = "2026-07-12T00:00:00Z"

	if got, want := buildVersion(), "v0.4.2 (abc1234) built 2026-07-12T00:00:00Z"; got != want {
		t.Fatalf("buildVersion() = %q, want %q", got, want)
	}
}
