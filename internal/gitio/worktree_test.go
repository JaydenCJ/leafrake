// Unit tests for the `git worktree list --porcelain` parser and the
// branch→worktree protection map.
package gitio

import "testing"

const worktreeFixture = `worktree /repo
HEAD 96af8e50431e99b7c27b01597bb08aad51b09936
branch refs/heads/main

worktree /repo-wt/feature
HEAD 28e88a674e88fef345bab178a213903afca37a7d
branch refs/heads/feature/login

worktree /repo-wt/detached
HEAD 1111111111111111111111111111111111111111
detached
`

func TestParseWorktreesThreeStanzas(t *testing.T) {
	list := ParseWorktrees([]byte(worktreeFixture))
	if len(list) != 3 {
		t.Fatalf("want 3 worktrees, got %d", len(list))
	}
	if list[0].Path != "/repo" || list[0].Branch != "main" || list[0].Detached {
		t.Fatalf("main worktree wrong: %+v", list[0])
	}
	if list[1].Branch != "feature/login" {
		t.Fatalf("branch not stripped to short name: %+v", list[1])
	}
	if !list[2].Detached || list[2].Branch != "" {
		t.Fatalf("detached worktree wrong: %+v", list[2])
	}
}

func TestParseWorktreesEmptyInput(t *testing.T) {
	if got := ParseWorktrees(nil); len(got) != 0 {
		t.Fatalf("empty input: got %v", got)
	}
}

func TestParseWorktreesNoTrailingBlankLine(t *testing.T) {
	// git ends output with a blank line, but the parser must not rely on it.
	raw := "worktree /only\nHEAD 2222222222222222222222222222222222222222\nbranch refs/heads/solo"
	list := ParseWorktrees([]byte(raw))
	if len(list) != 1 || list[0].Branch != "solo" {
		t.Fatalf("unterminated stanza wrong: %+v", list)
	}
}

func TestParseWorktreesPathMayContainSpaces(t *testing.T) {
	raw := "worktree /home/dev/my project\nHEAD 3333333333333333333333333333333333333333\nbranch refs/heads/x\n"
	list := ParseWorktrees([]byte(raw))
	if list[0].Path != "/home/dev/my project" {
		t.Fatalf("path with spaces mangled: %q", list[0].Path)
	}
}

func TestBranchWorktreesExcludesSelfAndDetached(t *testing.T) {
	list := ParseWorktrees([]byte(worktreeFixture))
	m := BranchWorktrees(list, "/repo")
	if _, ok := m["main"]; ok {
		t.Fatal("the scanning worktree's own branch must not be in the map")
	}
	if got := m["feature/login"]; got != "/repo-wt/feature" {
		t.Fatalf("feature/login: got %q", got)
	}
	if len(m) != 1 {
		t.Fatalf("want exactly 1 entry, got %v", m)
	}
}

func TestBranchWorktreesDifferentSelfKeepsAll(t *testing.T) {
	// When the scan runs from a linked worktree, the primary worktree's
	// branch must still be protected.
	list := ParseWorktrees([]byte(worktreeFixture))
	m := BranchWorktrees(list, "/repo-wt/feature")
	if got := m["main"]; got != "/repo" {
		t.Fatalf("main should map to /repo, got %q", got)
	}
	if _, ok := m["feature/login"]; ok {
		t.Fatal("self worktree branch must be excluded")
	}
}
