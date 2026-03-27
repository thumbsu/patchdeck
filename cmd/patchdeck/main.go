package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/thumbsu/patchdeck/internal/config"
	"github.com/thumbsu/patchdeck/internal/scanner"
	"github.com/thumbsu/patchdeck/internal/statusmodel"
	"github.com/thumbsu/patchdeck/internal/tui"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "use":
			if err := runUse(os.Args[2:]); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			return
		case "current":
			if err := runCurrent(); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			return
		case "list":
			if err := runListCommand(os.Args[2:]); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			return
		}
	}

	var repo string
	var listMode bool

	flag.StringVar(&repo, "repo", "", "path to a git repo root or worktree")
	flag.BoolVar(&listMode, "list", false, "print a text summary instead of starting the TUI")
	flag.Parse()

	resolvedRepo, err := resolveRepo(repo)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if listMode {
		if err := runList(resolvedRepo); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	if err := tui.Run(resolvedRepo); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runList(repo string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	root, refs, err := scanner.Discover(ctx, repo)
	if err != nil {
		return err
	}

	statuses := make([]statusmodel.WorktreeStatus, 0, len(refs))
	for _, ref := range refs {
		statuses = append(statuses, statusmodel.Load(ctx, ref))
	}

	sort.SliceStable(statuses, func(i, j int) bool {
		if statuses[i].Severity != statuses[j].Severity {
			return statuses[i].Severity > statuses[j].Severity
		}
		return len(statuses[i].ChangedFiles) > len(statuses[j].ChangedFiles)
	})

	fmt.Printf("patchdeck list\nrepo: %s\n\n", root)
	for _, status := range statuses {
		label := status.Ref.Branch
		if label == "" {
			label = status.Ref.Path
		}
		fmt.Printf("%-24s  files=%-3d  conflicts=%-2d  %s\n", label, len(status.ChangedFiles), status.ConflictedCount, status.ReasonLine)
	}
	return nil
}

func runUse(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: patchdeck use /absolute/path/to/repo")
	}

	repo, err := filepath.Abs(args[0])
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	root, _, err := scanner.Discover(ctx, repo)
	if err != nil {
		return fmt.Errorf("not a valid git repo/worktree: %w", err)
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg.DefaultRepo = root
	if err := config.Save(cfg); err != nil {
		return err
	}

	fmt.Printf("patchdeck default repo set: %s\n", root)
	return nil
}

func runCurrent() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.DefaultRepo == "" {
		return fmt.Errorf("no default repo configured; run: patchdeck use /path/to/repo")
	}
	fmt.Println(cfg.DefaultRepo)
	return nil
}

func runListCommand(args []string) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	var repo string
	fs.StringVar(&repo, "repo", "", "path to a git repo root or worktree")
	if err := fs.Parse(args); err != nil {
		return err
	}
	resolvedRepo, err := resolveRepo(repo)
	if err != nil {
		return err
	}
	return runList(resolvedRepo)
}

func resolveRepo(flagRepo string) (string, error) {
	if flagRepo != "" {
		return flagRepo, nil
	}

	// First try "where I'm standing now".
	if _, _, err := scanner.Discover(context.Background(), ""); err == nil {
		return "", nil
	}

	cfg, err := config.Load()
	if err != nil {
		return "", err
	}
	if cfg.DefaultRepo != "" {
		return cfg.DefaultRepo, nil
	}

	return "", fmt.Errorf("no repo detected in current directory and no default repo configured; run inside a repo or set one with: patchdeck use /path/to/repo")
}
