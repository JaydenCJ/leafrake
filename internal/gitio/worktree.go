package gitio

import "strings"

// Worktree is one entry of `git worktree list --porcelain`.
type Worktree struct {
	Path     string // absolute path of the worktree
	Head     string // commit hash checked out there
	Branch   string // short branch name, "" when detached
	Detached bool
}

// ParseWorktrees parses `git worktree list --porcelain` output: stanzas of
// "key value" lines separated by blank lines.
func ParseWorktrees(raw []byte) []Worktree {
	var (
		list []Worktree
		cur  Worktree
		open bool
	)
	flush := func() {
		if open {
			list = append(list, cur)
			cur = Worktree{}
			open = false
		}
	}
	for _, line := range strings.Split(string(raw), "\n") {
		switch {
		case line == "":
			flush()
		case strings.HasPrefix(line, "worktree "):
			flush()
			cur.Path = strings.TrimPrefix(line, "worktree ")
			open = true
		case strings.HasPrefix(line, "HEAD "):
			cur.Head = strings.TrimPrefix(line, "HEAD ")
		case strings.HasPrefix(line, "branch "):
			cur.Branch = strings.TrimPrefix(strings.TrimPrefix(line, "branch "), "refs/heads/")
		case line == "detached":
			cur.Detached = true
		}
	}
	flush()
	return list
}

// BranchWorktrees maps each branch name to the path of the worktree it is
// checked out in, excluding the worktree rooted at selfTop (the one the
// scan runs in — its branch is already HEAD-protected).
func BranchWorktrees(list []Worktree, selfTop string) map[string]string {
	m := make(map[string]string)
	for _, w := range list {
		if w.Branch == "" || w.Path == selfTop {
			continue
		}
		m[w.Branch] = w.Path
	}
	return m
}
