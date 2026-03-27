package scanner

import "testing"

func TestParsePorcelain(t *testing.T) {
	raw := `worktree /tmp/repo
HEAD aaaaaaaa
branch refs/heads/main

worktree /tmp/repo-feature
HEAD bbbbbbbb
branch refs/heads/feature/test

worktree /tmp/repo-broken
HEAD cccccccc
branch refs/heads/bad
prunable gitdir file points to non-existent location
`

	refs := parsePorcelain("/tmp/repo", raw)
	if len(refs) != 3 {
		t.Fatalf("expected 3 refs, got %d", len(refs))
	}

	if !refs[0].IsCurrent || refs[0].Branch != "main" {
		t.Fatalf("unexpected first ref: %#v", refs[0])
	}

	if refs[1].Branch != "feature/test" {
		t.Fatalf("unexpected second branch: %#v", refs[1])
	}

	if !refs[2].Prunable {
		t.Fatalf("expected prunable ref: %#v", refs[2])
	}
}

func TestParsePorcelainKeepsDotPrefixedWorktrees(t *testing.T) {
	raw := `worktree /tmp/repo
HEAD aaaaaaaa
branch refs/heads/main

worktree /tmp/repo/.claude/worktrees/review-agent
HEAD bbbbbbbb
branch refs/heads/review-agent
`

	refs := parsePorcelain("/tmp/repo", raw)
	if len(refs) != 2 {
		t.Fatalf("expected 2 refs, got %d", len(refs))
	}

	if refs[1].Path != "/tmp/repo/.claude/worktrees/review-agent" {
		t.Fatalf("dot-prefixed worktree path was altered or filtered: %#v", refs[1])
	}

	if refs[1].Branch != "review-agent" {
		t.Fatalf("unexpected branch for dot-prefixed worktree: %#v", refs[1])
	}
}
