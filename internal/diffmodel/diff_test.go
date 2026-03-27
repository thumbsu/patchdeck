package diffmodel

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thumbsu/patchdeck/internal/statusmodel"
)

func TestLoadDirectoryPreview(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".claude", "worktrees", "agent-a")
	if err := os.MkdirAll(filepath.Join(dir, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "nested", "note.txt"), []byte("world"), 0o644); err != nil {
		t.Fatal(err)
	}

	file := statusmodel.ChangedFile{
		Path:  ".claude/worktrees/agent-a",
		IsDir: true,
	}

	preview, err := Load(context.Background(), root, file)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(preview.Header, "[directory]") {
		t.Fatalf("expected directory header, got %q", preview.Header)
	}
	if !strings.Contains(preview.PatchText, "README.md") {
		t.Fatalf("expected directory summary to include files, got %q", preview.PatchText)
	}
	if !strings.Contains(preview.PatchText, "nested/") {
		t.Fatalf("expected nested directory summary, got %q", preview.PatchText)
	}
}
