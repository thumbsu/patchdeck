package commitmodel

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/thumbsu/patchdeck/internal/gitutil"
)

type Commit struct {
	Hash      string
	ShortHash string
	UnixTime  int64
	Subject   string
	Relative  string
}

type CommitFile struct {
	Path       string
	BaseName   string
	Dir        string
	StatusCode string
}

func Load(ctx context.Context, worktreePath string) ([]Commit, error) {
	rangeSpec, err := commitRange(ctx, worktreePath)
	if err != nil {
		return nil, err
	}

	args := []string{
		"log",
		"--date-order",
		"--format=%H%x1f%h%x1f%ct%x1f%s",
		"-n", "50",
	}
	if rangeSpec != "" {
		args = append(args, rangeSpec)
	} else {
		args = append(args, "HEAD")
	}

	out, err := gitutil.RunGit(ctx, worktreePath, args...)
	if err != nil {
		return nil, err
	}

	return parseLog(out, time.Now()), nil
}

func commitRange(ctx context.Context, worktreePath string) (string, error) {
	base := detectBase(ctx, worktreePath)
	if base == "" {
		return "", nil
	}

	mergeBase, err := gitutil.RunGit(ctx, worktreePath, "merge-base", "HEAD", base)
	if err != nil {
		return "", nil
	}

	mergeBase = strings.TrimSpace(mergeBase)
	if mergeBase == "" {
		return "", nil
	}

	return mergeBase + "..HEAD", nil
}

func detectBase(ctx context.Context, worktreePath string) string {
	if out, err := gitutil.RunGitAllowExitCodes(ctx, worktreePath, []int{128}, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}"); err == nil {
		upstream := strings.TrimSpace(out)
		if upstream != "" && upstream != "@{upstream}" {
			return upstream
		}
	}

	if out, err := gitutil.RunGitAllowExitCodes(ctx, worktreePath, []int{128}, "symbolic-ref", "refs/remotes/origin/HEAD"); err == nil {
		sym := strings.TrimSpace(out)
		if sym != "" {
			return strings.TrimPrefix(sym, "refs/remotes/")
		}
	}

	for _, candidate := range []string{"main", "master"} {
		if _, err := gitutil.RunGitAllowExitCodes(ctx, worktreePath, []int{128}, "rev-parse", "--verify", candidate); err == nil {
			return candidate
		}
	}

	return ""
}

func parseLog(raw string, now time.Time) []Commit {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	commits := make([]Commit, 0)
	for _, line := range strings.Split(raw, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.SplitN(line, "\x1f", 4)
		if len(parts) != 4 {
			continue
		}

		unixTime, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			continue
		}

		commits = append(commits, Commit{
			Hash:      parts[0],
			ShortHash: parts[1],
			UnixTime:  unixTime,
			Subject:   parts[3],
			Relative:  relativeTime(now.Sub(time.Unix(unixTime, 0))),
		})
	}

	return commits
}

func LoadFiles(ctx context.Context, worktreePath string, commitHash string) ([]CommitFile, error) {
	out, err := gitutil.RunGit(ctx, worktreePath, "show", "--format=", "--name-status", "--no-renames", commitHash)
	if err != nil {
		return nil, err
	}

	files := make([]CommitFile, 0)
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}

		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}

		path := strings.TrimSpace(parts[1])
		files = append(files, CommitFile{
			Path:       path,
			BaseName:   filepath.Base(path),
			Dir:        filepath.Dir(path),
			StatusCode: strings.TrimSpace(parts[0]),
		})
	}

	return files, nil
}

func relativeTime(d time.Duration) string {
	if d < time.Minute {
		return "just now"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	if d < 30*24*time.Hour {
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
	if d < 365*24*time.Hour {
		return fmt.Sprintf("%dmo ago", int(d.Hours()/(24*30)))
	}
	return fmt.Sprintf("%dy ago", int(d.Hours()/(24*365)))
}
