# patchdeck

Review-first console for multi-worktree Git workflows.

## What It Is

`patchdeck` is a local TUI/CLI for reviewing changes across multiple Git worktrees.
It is designed for terminal and Vim-oriented developers who need to inspect AI-agent
or parallel-branch edits without jumping between directories and IDE panes.

The core flow is:

1. Pick the worktree that needs attention first.
2. Inspect changed files inside that worktree.
3. Read the diff preview.
4. Jump into the original worktree or editor.

## Visibility Rule

If a path is returned by `git worktree`, `patchdeck` shows it.

That includes worktrees rooted under dot-prefixed directories such as `.claude/`.
`patchdeck` does not hide paths just because they look like config or internal folders.
If Git considers the worktree real, it is reviewable.

## Design Direction

- Language: Go
- TUI: Bubble Tea
- Product posture: calm, dense, utility-first review console
- Primary audience: Vim/terminal users working in monorepos with multiple worktrees

## Project Layout

- `cmd/patchdeck`: CLI entrypoint
- `internal/scanner`: worktree discovery
- `internal/statusmodel`: normalized worktree and file status
- `internal/diffmodel`: diff preview generation
- `internal/navigation`: editor/worktree jump intents
- `internal/tui`: Bubble Tea adapter and UI state
- `docs/`: design docs and implementation notes
- `assets/`: sketches and reference artifacts

## Next Build Step

Implement a CLI spike that lists worktrees and their normalized dirty/conflict status
before building the TUI.

## Usage

Quick start:

```bash
git clone git@github-personal:thumbsu/patchdeck.git
cd patchdeck
./scripts/install.sh
patchdeck use /absolute/path/to/repo
patchdeck
```

Run the TUI:

```bash
patchdeck
```

If you're not standing inside a repo, set a default once:

```bash
patchdeck use /absolute/path/to/repo
patchdeck
```

Print a non-interactive summary:

```bash
patchdeck list
```

Build a local binary:

```bash
make build
./patchdeck --repo /absolute/path/to/repo
```

Install locally so `patchdeck` is on your PATH:

```bash
make install
```

Or:

```bash
./scripts/install.sh
```

Install on another computer:

```bash
git clone git@github-personal:thumbsu/patchdeck.git
cd patchdeck
./scripts/install.sh
patchdeck use /absolute/path/to/repo
patchdeck
```

HTTPS clone:

```bash
git clone https://github.com/thumbsu/patchdeck.git
```

## Releases

Tagged builds publish release binaries for:

- macOS arm64
- macOS amd64
- Linux amd64
- Linux arm64

If you prefer downloading a binary instead of building locally, use the latest
GitHub Release from:

- https://github.com/thumbsu/patchdeck/releases

Key interactions:

- `f`: current uncommitted file changes
- `c`: branch commit history
- `j/k`: move
- `h/l` or `Enter`: drill or switch pane
- `r`: refresh
- `n/N`: jump across priority items in the center pane
