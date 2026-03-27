package statusmodel

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/lu1ee/patchdeck/internal/gitutil"
	"github.com/lu1ee/patchdeck/internal/scanner"
)

type Severity int

const (
	SeverityClean Severity = iota
	SeverityDirty
	SeverityBusy
	SeverityConflict
	SeverityError
)

type ChangedFile struct {
	Path       string
	BaseName   string
	Dir        string
	StatusCode string
	StagedCode byte
	WorkCode   byte
	Untracked  bool
	Conflicted bool
	Deleted    bool
	IsDir      bool
}

type WorktreeStatus struct {
	Ref             scanner.WorktreeRef
	Dirty           bool
	StagedCount     int
	UnstagedCount   int
	UntrackedCount  int
	ConflictedCount int
	ChangedFiles    []ChangedFile
	Loaded          bool
	Loading         bool
	ScanError       string
	ReasonLine      string
	Severity        Severity
}

func Load(ctx context.Context, ref scanner.WorktreeRef) WorktreeStatus {
	if ref.Prunable {
		return WorktreeStatus{
			Ref:        ref,
			Loaded:     true,
			ScanError:  strings.TrimSpace(ref.PrunableReason),
			ReasonLine: "broken worktree needs attention",
			Severity:   SeverityError,
		}
	}

	out, err := gitutil.RunGit(ctx, ref.Path, "status", "--short", "--branch", "--untracked-files=all")
	if err != nil {
		return WorktreeStatus{
			Ref:        ref,
			Loaded:     true,
			ScanError:  err.Error(),
			ReasonLine: "scan failed",
			Severity:   SeverityError,
		}
	}

	status := parseStatus(ref, out)
	enrichPaths(ref.Path, &status)
	status.Loaded = true
	return status
}

func parseStatus(ref scanner.WorktreeRef, raw string) WorktreeStatus {
	lines := strings.Split(strings.TrimRight(raw, "\n"), "\n")
	files := make([]ChangedFile, 0, len(lines))
	staged := 0
	unstaged := 0
	untracked := 0
	conflicted := 0

	for _, line := range lines {
		if line == "" || strings.HasPrefix(line, "## ") {
			continue
		}

		if len(line) < 3 {
			continue
		}

		x := line[0]
		y := line[1]
		pathText := strings.TrimSpace(line[3:])
		if strings.Contains(pathText, " -> ") {
			parts := strings.Split(pathText, " -> ")
			pathText = parts[len(parts)-1]
		}

		pathText = filepath.Clean(pathText)
		file := ChangedFile{
			Path:       pathText,
			BaseName:   filepath.Base(pathText),
			Dir:        filepath.Dir(pathText),
			StatusCode: string([]byte{x, y}),
			StagedCode: x,
			WorkCode:   y,
			Untracked:  x == '?' && y == '?',
			Conflicted: isConflicted(x, y),
			Deleted:    x == 'D' || y == 'D',
		}

		if file.Untracked {
			untracked++
		}
		if file.Conflicted {
			conflicted++
		}
		if x != ' ' && x != '?' {
			staged++
		}
		if y != ' ' && y != '?' {
			unstaged++
		}

		files = append(files, file)
	}

	sort.SliceStable(files, func(i, j int) bool {
		left := filePriority(files[i])
		right := filePriority(files[j])
		if left != right {
			return left > right
		}
		if files[i].BaseName != files[j].BaseName {
			return files[i].BaseName < files[j].BaseName
		}
		return files[i].Path < files[j].Path
	})

	status := WorktreeStatus{
		Ref:             ref,
		Dirty:           len(files) > 0,
		StagedCount:     staged,
		UnstagedCount:   unstaged,
		UntrackedCount:  untracked,
		ConflictedCount: conflicted,
		ChangedFiles:    files,
		Severity:        SeverityClean,
		ReasonLine:      "clean",
	}

	total := len(files)
	switch {
	case conflicted > 0:
		status.Severity = SeverityConflict
		status.ReasonLine = reasonLine(conflicted, total)
	case total > 0:
		status.Severity = SeverityBusy
		status.ReasonLine = reasonLine(conflicted, total)
	default:
		status.Severity = SeverityClean
		status.ReasonLine = "review queue is clear"
	}

	return status
}

func isConflicted(x, y byte) bool {
	token := string([]byte{x, y})
	switch token {
	case "DD", "AU", "UD", "UA", "DU", "AA", "UU":
		return true
	default:
		return false
	}
}

func filePriority(file ChangedFile) int {
	switch {
	case file.Conflicted:
		return 4
	case file.Untracked:
		return 3
	case file.Deleted:
		return 2
	default:
		return 1
	}
}

func reasonLine(conflicted, total int) string {
	switch {
	case conflicted > 0:
		return plural(conflicted, "conflict") + ", " + plural(total, "changed file")
	case total > 0:
		return plural(total, "changed file")
	default:
		return "review queue is clear"
	}
}

func plural(count int, noun string) string {
	if count == 1 {
		return "1 " + noun
	}
	return strings.TrimSpace(strings.Join([]string{itoa(count), noun + "s"}, " "))
}

func itoa(n int) string {
	return strconv.Itoa(n)
}

func enrichPaths(worktreePath string, status *WorktreeStatus) {
	for i := range status.ChangedFiles {
		file := &status.ChangedFiles[i]
		if file.Deleted {
			continue
		}
		info, err := os.Stat(filepath.Join(worktreePath, file.Path))
		if err != nil {
			continue
		}
		file.IsDir = info.IsDir()
		if file.IsDir {
			file.BaseName = filepath.Base(file.Path) + "/"
		}
	}
}
