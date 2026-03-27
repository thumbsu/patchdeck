package scanner

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/lu1ee/patchdeck/internal/gitutil"
)

type WorktreeRef struct {
	Path           string
	Branch         string
	HeadSHA        string
	IsBare         bool
	IsCurrent      bool
	Prunable       bool
	PrunableReason string
}

func Discover(ctx context.Context, repoArg string) (string, []WorktreeRef, error) {
	base := repoArg
	if base == "" {
		var err error
		base, err = os.Getwd()
		if err != nil {
			return "", nil, err
		}
	}

	rootOut, err := gitutil.RunGit(ctx, base, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", nil, err
	}

	root := strings.TrimSpace(rootOut)
	root = filepath.Clean(root)

	out, err := gitutil.RunGit(ctx, root, "worktree", "list", "--porcelain")
	if err != nil {
		return "", nil, err
	}

	refs := parsePorcelain(root, out)
	return root, refs, nil
}

func parsePorcelain(root string, raw string) []WorktreeRef {
	blocks := strings.Split(strings.TrimSpace(raw), "\n\n")
	refs := make([]WorktreeRef, 0, len(blocks))

	for _, block := range blocks {
		if strings.TrimSpace(block) == "" {
			continue
		}

		ref := WorktreeRef{}
		lines := strings.Split(block, "\n")
		for _, line := range lines {
			switch {
			case strings.HasPrefix(line, "worktree "):
				ref.Path = filepath.Clean(strings.TrimPrefix(line, "worktree "))
			case strings.HasPrefix(line, "HEAD "):
				ref.HeadSHA = strings.TrimPrefix(line, "HEAD ")
			case strings.HasPrefix(line, "branch "):
				branch := strings.TrimPrefix(line, "branch ")
				ref.Branch = strings.TrimPrefix(branch, "refs/heads/")
			case strings.HasPrefix(line, "detached"):
				ref.Branch = "(detached)"
			case strings.HasPrefix(line, "bare"):
				ref.IsBare = true
			case strings.HasPrefix(line, "prunable"):
				ref.Prunable = true
				ref.PrunableReason = strings.TrimSpace(strings.TrimPrefix(line, "prunable"))
			}
		}

		ref.IsCurrent = filepath.Clean(ref.Path) == root
		refs = append(refs, ref)
	}

	return refs
}
