package tui

import (
	"testing"

	"github.com/thumbsu/patchdeck/internal/commitmodel"
	"github.com/thumbsu/patchdeck/internal/diffmodel"
	"github.com/thumbsu/patchdeck/internal/scanner"
	"github.com/thumbsu/patchdeck/internal/statusmodel"
)

func TestSelectCommitClearsStaleCommitFileSelection(t *testing.T) {
	m := New("")
	m.selectedWorktreePath = "/tmp/repo"
	m.centerMode = centerCommitFiles
	m.refs["/tmp/repo"] = scanner.WorktreeRef{Path: "/tmp/repo", Branch: "feature/a"}
	m.statuses["/tmp/repo"] = statusmodel.WorktreeStatus{
		Ref: scanner.WorktreeRef{Path: "/tmp/repo", Branch: "feature/a"},
	}
	m.selectedCommitHash = "old"
	m.selectedCommitFilePath = "old/file.go"
	m.diff = diffmodel.DiffPreview{Header: "stale"}
	m.diffViewKey = "commit-file:old:old/file.go"
	m.commits["/tmp/repo"] = []commitmodel.Commit{
		{Hash: "new"},
	}
	m.commitFiles[commitFilesKey("/tmp/repo", "new")] = []commitmodel.CommitFile{
		{Path: "new/file.go"},
	}

	next, cmd := m.selectCommit("new")
	if cmd == nil {
		t.Fatal("expected diff/load command for new commit")
	}
	if next.selectedCommitHash != "new" {
		t.Fatalf("expected selected commit to change, got %q", next.selectedCommitHash)
	}
	if next.selectedCommitFilePath != "new/file.go" {
		t.Fatalf("expected first commit file to be selected, got %q", next.selectedCommitFilePath)
	}
	if next.diffViewKey != "" {
		t.Fatalf("expected stale diff key to clear, got %q", next.diffViewKey)
	}
}

func TestEnsureSelectionPicksFirstCommitFile(t *testing.T) {
	m := New("")
	m.selectedWorktreePath = "/tmp/repo"
	m.refs["/tmp/repo"] = scanner.WorktreeRef{Path: "/tmp/repo", Branch: "feature/a"}
	m.commits["/tmp/repo"] = []commitmodel.Commit{
		{Hash: "abc123"},
	}
	m.commitFiles[commitFilesKey("/tmp/repo", "abc123")] = []commitmodel.CommitFile{
		{Path: "first.go"},
		{Path: "second.go"},
	}
	m.statuses["/tmp/repo"] = statusmodel.WorktreeStatus{
		Ref:          scanner.WorktreeRef{Path: "/tmp/repo", Branch: "feature/a"},
		ChangedFiles: []statusmodel.ChangedFile{{Path: "worktree.go"}},
	}

	m.ensureSelection()

	if m.selectedCommitHash != "abc123" {
		t.Fatalf("expected first commit selected, got %q", m.selectedCommitHash)
	}
	if m.selectedCommitFilePath != "first.go" {
		t.Fatalf("expected first commit file selected, got %q", m.selectedCommitFilePath)
	}
}

func TestCurrentOpenPathUsesCommitFileInCommitFileMode(t *testing.T) {
	m := New("")
	m.centerMode = centerCommitFiles
	m.selectedWorktreePath = "/tmp/repo"
	m.selectedCommitHash = "abc123"
	m.commitFiles[commitFilesKey("/tmp/repo", "abc123")] = []commitmodel.CommitFile{
		{Path: "commit/file.go"},
	}

	path, ok := m.currentOpenPath()
	if !ok {
		t.Fatal("expected open path")
	}
	if path != "commit/file.go" {
		t.Fatalf("unexpected open path: %q", path)
	}
}

func TestFollowOffsetOnlyMovesWhenSelectionLeavesViewport(t *testing.T) {
	offset := 0
	offset = followOffset(offset, 2, 5)
	if offset != 0 {
		t.Fatalf("expected offset unchanged, got %d", offset)
	}
	offset = followOffset(offset, 5, 5)
	if offset != 1 {
		t.Fatalf("expected offset to advance by one, got %d", offset)
	}
	offset = followOffset(offset, 3, 5)
	if offset != 1 {
		t.Fatalf("expected offset to stay stable, got %d", offset)
	}
}

func TestSelectWorktreeSelectsFirstFile(t *testing.T) {
	m := New("")
	m.centerMode = centerFiles
	m.refs["/tmp/repo"] = scanner.WorktreeRef{Path: "/tmp/repo", Branch: "feature/a"}
	m.statuses["/tmp/repo"] = statusmodel.WorktreeStatus{
		Ref: scanner.WorktreeRef{Path: "/tmp/repo", Branch: "feature/a"},
		ChangedFiles: []statusmodel.ChangedFile{
			{Path: "first.go"},
			{Path: "second.go"},
		},
	}

	next, cmd := m.selectWorktree("/tmp/repo")
	if cmd == nil {
		t.Fatal("expected diff command")
	}
	if next.selectedFilePath != "first.go" {
		t.Fatalf("expected first file selected, got %q", next.selectedFilePath)
	}
}
