package diffmodel

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lu1ee/patchdeck/internal/commitmodel"
	"github.com/lu1ee/patchdeck/internal/gitutil"
	"github.com/lu1ee/patchdeck/internal/statusmodel"
)

const (
	MaxPreviewBytes = 64 * 1024
	MaxPreviewLines = 400
)

type DiffPreview struct {
	FilePath  string
	Header    string
	PatchText string
	Truncated bool
	TooLarge  bool
	LineCount int
}

func Load(ctx context.Context, worktreePath string, file statusmodel.ChangedFile) (DiffPreview, error) {
	if file.IsDir {
		return directoryPreview(worktreePath, file.Path)
	}

	patch, err := patchForFile(ctx, worktreePath, file)
	if err != nil {
		return DiffPreview{}, err
	}

	lines := strings.Split(strings.TrimRight(patch, "\n"), "\n")
	truncated := false
	tooLarge := false

	if len([]byte(patch)) > MaxPreviewBytes {
		patch = string([]byte(patch)[:MaxPreviewBytes])
		tooLarge = true
		truncated = true
		lines = strings.Split(strings.TrimRight(patch, "\n"), "\n")
	}
	if len(lines) > MaxPreviewLines {
		lines = lines[:MaxPreviewLines]
		patch = strings.Join(lines, "\n")
		truncated = true
		tooLarge = true
	}

	header := file.Path
	if tooLarge {
		header += "  [truncated]"
	}

	return DiffPreview{
		FilePath:  file.Path,
		Header:    header,
		PatchText: patch,
		Truncated: truncated,
		TooLarge:  tooLarge,
		LineCount: len(lines),
	}, nil
}

func LoadCommit(ctx context.Context, worktreePath string, commit commitmodel.Commit) (DiffPreview, error) {
	out, err := gitutil.RunGit(ctx, worktreePath, "show", "--no-ext-diff", "--color=never", "--stat", "--patch", "--format=fuller", commit.Hash)
	if err != nil {
		return DiffPreview{}, err
	}

	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	truncated := false
	tooLarge := false

	if len([]byte(out)) > MaxPreviewBytes {
		out = string([]byte(out)[:MaxPreviewBytes])
		tooLarge = true
		truncated = true
		lines = strings.Split(strings.TrimRight(out, "\n"), "\n")
	}
	if len(lines) > MaxPreviewLines {
		lines = lines[:MaxPreviewLines]
		out = strings.Join(lines, "\n")
		truncated = true
		tooLarge = true
	}

	header := commit.ShortHash + "  " + commit.Subject
	if tooLarge {
		header += "  [truncated]"
	}

	return DiffPreview{
		FilePath:  commit.Hash,
		Header:    header,
		PatchText: out,
		Truncated: truncated,
		TooLarge:  tooLarge,
		LineCount: len(lines),
	}, nil
}

func LoadCommitFile(ctx context.Context, worktreePath string, commit commitmodel.Commit, file commitmodel.CommitFile) (DiffPreview, error) {
	out, err := gitutil.RunGit(ctx, worktreePath, "show", "--no-ext-diff", "--color=never", commit.Hash, "--", file.Path)
	if err != nil {
		return DiffPreview{}, err
	}

	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	truncated := false
	tooLarge := false

	if len([]byte(out)) > MaxPreviewBytes {
		out = string([]byte(out)[:MaxPreviewBytes])
		tooLarge = true
		truncated = true
		lines = strings.Split(strings.TrimRight(out, "\n"), "\n")
	}
	if len(lines) > MaxPreviewLines {
		lines = lines[:MaxPreviewLines]
		out = strings.Join(lines, "\n")
		truncated = true
		tooLarge = true
	}

	header := commit.ShortHash + "  " + file.Path
	if tooLarge {
		header += "  [truncated]"
	}

	return DiffPreview{
		FilePath:  file.Path,
		Header:    header,
		PatchText: out,
		Truncated: truncated,
		TooLarge:  tooLarge,
		LineCount: len(lines),
	}, nil
}

func patchForFile(ctx context.Context, worktreePath string, file statusmodel.ChangedFile) (string, error) {
	if file.Untracked {
		full := filepath.Join(worktreePath, file.Path)
		out, err := gitutil.RunGitAllowExitCodes(ctx, worktreePath, []int{1}, "diff", "--no-ext-diff", "--no-index", "--", "/dev/null", full)
		if err != nil {
			return "", err
		}
		return out, nil
	}

	out, err := gitutil.RunGit(ctx, worktreePath, "diff", "--no-ext-diff", "HEAD", "--", file.Path)
	if err == nil && strings.TrimSpace(out) != "" {
		return out, nil
	}

	if file.Deleted {
		return fmt.Sprintf("deleted file: %s", file.Path), nil
	}

	full := filepath.Join(worktreePath, file.Path)
	if _, statErr := os.Stat(full); statErr == nil {
		return fmt.Sprintf("no patch available for %s", file.Path), nil
	}

	return "", err
}

func directoryPreview(worktreePath string, relative string) (DiffPreview, error) {
	root := filepath.Join(worktreePath, relative)
	entries := []string{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(worktreePath, path)
		if err != nil {
			return err
		}
		suffix := ""
		if d.IsDir() {
			suffix = "/"
		}
		entries = append(entries, rel+suffix)
		if len(entries) >= 40 {
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return DiffPreview{}, err
	}

	body := "directory item returned by git status\n\n"
	body += "This path is a directory, so patchdeck shows a directory summary instead of a line diff.\n\n"
	if len(entries) == 0 {
		body += "(empty directory)"
	} else {
		body += strings.Join(entries, "\n")
	}

	return DiffPreview{
		FilePath:  relative,
		Header:    relative + "  [directory]",
		PatchText: body,
		LineCount: len(strings.Split(body, "\n")),
	}, nil
}
