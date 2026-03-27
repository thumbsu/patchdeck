package commitmodel

import (
	"testing"
	"time"
)

func TestParseLog(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	raw := "aaaaaaaa\x1faaaaaaa\x1f1799996400\x1ffirst commit\nbbbbbbbb\x1fbbbbbbb\x1f1799913600\x1fsecond commit"

	commits := parseLog(raw, now)
	if len(commits) != 2 {
		t.Fatalf("expected 2 commits, got %d", len(commits))
	}
	if commits[0].ShortHash != "aaaaaaa" || commits[0].Subject != "first commit" {
		t.Fatalf("unexpected first commit: %#v", commits[0])
	}
	if commits[0].Relative == "" {
		t.Fatalf("expected relative time")
	}
}
