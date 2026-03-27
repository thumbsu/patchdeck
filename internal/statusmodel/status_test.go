package statusmodel

import (
	"testing"

	"github.com/thumbsu/patchdeck/internal/scanner"
)

func TestParseStatus(t *testing.T) {
	ref := scanner.WorktreeRef{Path: "/tmp/repo", Branch: "feature/a"}
	raw := `## feature/a
UU internal/core/file.go
 M README.md
?? scratch.txt
D  old.txt
`

	status := parseStatus(ref, raw)
	if !status.Dirty {
		t.Fatal("expected dirty status")
	}
	if status.ConflictedCount != 1 {
		t.Fatalf("expected 1 conflict, got %d", status.ConflictedCount)
	}
	if status.UntrackedCount != 1 {
		t.Fatalf("expected 1 untracked, got %d", status.UntrackedCount)
	}
	if status.Severity != SeverityConflict {
		t.Fatalf("expected conflict severity, got %v", status.Severity)
	}
	if len(status.ChangedFiles) != 4 {
		t.Fatalf("expected 4 files, got %d", len(status.ChangedFiles))
	}
	if !status.ChangedFiles[0].Conflicted {
		t.Fatalf("expected conflicted file first, got %#v", status.ChangedFiles[0])
	}
}

func TestParseStatusReasonLine(t *testing.T) {
	ref := scanner.WorktreeRef{Path: "/tmp/repo", Branch: "feature/a"}
	raw := `## feature/a
UU internal/core/file.go
 M README.md
`

	status := parseStatus(ref, raw)
	if status.ReasonLine != "1 conflict, 2 changed files" {
		t.Fatalf("unexpected reason line: %q", status.ReasonLine)
	}
}
