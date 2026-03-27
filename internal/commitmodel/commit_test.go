package commitmodel

import (
	"testing"
	"time"
)

func TestParseLog(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	raw := "* \x1eaaaaaaaa\x1faaaaaaa\x1f1799996400\x1fbbbbbbbb cccccccc\x1ffirst commit\n| * \x1ebbbbbbbb\x1fbbbbbbb\x1f1799913600\x1fdddddddd\x1fsecond commit\n|/  "

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
	if len(commits[0].Parents) != 2 || commits[0].Parents[0] != "bbbbbbbb" || commits[0].Parents[1] != "cccccccc" {
		t.Fatalf("unexpected first commit parents: %#v", commits[0].Parents)
	}
	if len(commits[1].Parents) != 1 || commits[1].Parents[0] != "dddddddd" {
		t.Fatalf("unexpected second commit parents: %#v", commits[1].Parents)
	}
	if commits[0].Graph != "*" {
		t.Fatalf("unexpected first graph prefix: %q", commits[0].Graph)
	}
	if commits[1].Graph != "| *" {
		t.Fatalf("unexpected second graph prefix: %q", commits[1].Graph)
	}
}
