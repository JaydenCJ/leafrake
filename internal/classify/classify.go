// Package classify turns per-branch facts into a verdict with quotable
// evidence. It is pure: no git, no clock, no I/O — the caller supplies
// every fact and the current time, which makes each rule unit-testable.
package classify

import (
	"fmt"
	"path"
	"sort"
	"time"
)

// Category is the primary classification of a branch. Exactly one applies;
// secondary findings (e.g. a merged branch whose upstream is also gone)
// appear as extra evidence lines instead.
type Category string

const (
	// Merged: the branch tip is reachable from base — a regular merge or
	// fast-forward landed it. git itself agrees (`branch --merged`).
	Merged Category = "merged"
	// Squashed: the branch's entire diff matches a single squash commit on
	// base. `branch --merged` misses these; leafrake proves them by patch-id.
	Squashed Category = "squash-merged"
	// Gone: the configured upstream was deleted on the remote, but the local
	// content could not be proven merged. Deleting needs an explicit opt-in.
	Gone Category = "gone"
	// Stale: no proof of merging, upstream intact or unset, and the last
	// commit is at least the stale threshold old.
	Stale Category = "stale"
	// Active: everything else — recent, unmerged work. Never deletable.
	Active Category = "active"
	// Protected: base, HEAD, checked out in a worktree, or --protect match.
	Protected Category = "protected"
)

// Order returns the display rank of a category (most-deletable first).
func Order(c Category) int {
	switch c {
	case Merged:
		return 0
	case Squashed:
		return 1
	case Gone:
		return 2
	case Stale:
		return 3
	case Active:
		return 4
	default:
		return 5
	}
}

// Facts is everything the engine gathered about one branch. Relation fields
// (MergeBase, Ahead, ...) are only populated for branches that were not
// skipped as protected.
type Facts struct {
	Name          string
	Tip           string // full hash
	IsHead        bool
	WorktreePath  string // non-empty: checked out in another worktree
	Upstream      string // "" when no upstream configured
	UpstreamGone  bool
	Track         string // raw tracking state, e.g. "[ahead 2]" or "[gone]"
	CommitterDate time.Time
	Subject       string

	MergeBase        string // "" together with NoCommonAncestor=false means "not computed"
	NoCommonAncestor bool
	IsAncestorOfBase bool
	EmptyDiff        bool   // diff(merge-base, tip) is empty
	PatchID          string // patch-id of the branch squashed into one commit
	Ahead            int
	Behind           int
}

// SquashMatch is the proof for a squash-merged branch: the exact commit on
// base whose patch-id equals the branch's squashed diff.
type SquashMatch struct {
	Hash    string
	Subject string
	When    time.Time
}

// BaseIndex maps patch-id → the base commit that carries it, built from one
// `git log -p | git patch-id --stable` pass over the base branch.
type BaseIndex map[string]SquashMatch

// Options tunes the pure classification rules.
type Options struct {
	BaseName  string
	StaleDays int       // a branch this many days or more untouched is stale
	Protect   []string  // extra protected patterns (path.Match globs or exact names)
	Now       time.Time // injected clock, for determinism
}

// Verdict is the classification of one branch plus its evidence.
type Verdict struct {
	Facts    Facts
	Category Category
	// Deletable is true only for the categories leafrake will delete by
	// default: Merged and Squashed, where the content provably lives on base.
	Deletable bool
	Squash    *SquashMatch
	Reasons   []string // human-readable evidence lines, in display order
}

// Decide classifies one branch. Precedence: protection first (base, HEAD,
// worktrees, --protect), then proof of merging (ancestor, empty diff,
// squash patch-id), then upstream state, then age.
func Decide(f Facts, idx BaseIndex, opt Options) Verdict {
	v := Verdict{Facts: f}

	// 1. Protection — never classify these further.
	switch {
	case f.Name == opt.BaseName:
		return protect(v, "this is the base branch")
	case f.IsHead:
		return protect(v, "currently checked out (HEAD)")
	case f.WorktreePath != "":
		return protect(v, fmt.Sprintf("checked out in worktree %s", f.WorktreePath))
	}
	if pat, ok := matchProtect(f.Name, opt.Protect); ok {
		return protect(v, fmt.Sprintf("matches --protect %q", pat))
	}

	// 2. Proof of merging.
	if f.IsAncestorOfBase {
		v.Category = Merged
		v.Deletable = true
		v.Reasons = append(v.Reasons,
			fmt.Sprintf("tip %.7s is an ancestor of %s", f.Tip, opt.BaseName))
		v.Reasons = append(v.Reasons, secondary(f, opt)...)
		return v
	}
	if !f.NoCommonAncestor && f.MergeBase != "" && f.EmptyDiff {
		v.Category = Merged
		v.Deletable = true
		v.Reasons = append(v.Reasons, fmt.Sprintf(
			"branch introduces no changes: tree identical to merge-base %.7s (net-zero diff)",
			f.MergeBase))
		v.Reasons = append(v.Reasons, secondary(f, opt)...)
		return v
	}
	if f.PatchID != "" {
		if m, ok := idx[f.PatchID]; ok {
			match := m
			v.Category = Squashed
			v.Deletable = true
			v.Squash = &match
			v.Reasons = append(v.Reasons,
				fmt.Sprintf("diff vs merge-base %.7s has patch-id %.16s…", f.MergeBase, f.PatchID),
				fmt.Sprintf("matches squash commit %.7s %q (%s) on %s",
					m.Hash, m.Subject, m.When.Format("2006-01-02"), opt.BaseName))
			v.Reasons = append(v.Reasons, secondary(f, opt)...)
			return v
		}
	}

	// 3. Upstream state.
	if f.UpstreamGone {
		v.Category = Gone
		v.Reasons = append(v.Reasons,
			fmt.Sprintf("upstream %s was deleted on the remote", f.Upstream),
			fmt.Sprintf("content not proven merged — %s", relation(f, opt)))
		return v
	}

	// 4. Age.
	age := ageDays(f.CommitterDate, opt.Now)
	if opt.StaleDays > 0 && age >= opt.StaleDays {
		v.Category = Stale
		v.Reasons = append(v.Reasons,
			fmt.Sprintf("last commit %s (%d days ago), stale threshold %d days",
				f.CommitterDate.Format("2006-01-02"), age, opt.StaleDays),
			relation(f, opt))
		return v
	}

	v.Category = Active
	v.Reasons = append(v.Reasons,
		fmt.Sprintf("%s; last commit %d days ago", relation(f, opt), age))
	return v
}

// DecideAll classifies every branch and sorts the verdicts by category rank
// then name, so output order is stable and the deletable rows lead.
func DecideAll(facts []Facts, idx BaseIndex, opt Options) []Verdict {
	verdicts := make([]Verdict, len(facts))
	for i, f := range facts {
		verdicts[i] = Decide(f, idx, opt)
	}
	sort.Slice(verdicts, func(i, j int) bool {
		oi, oj := Order(verdicts[i].Category), Order(verdicts[j].Category)
		if oi != oj {
			return oi < oj
		}
		return verdicts[i].Facts.Name < verdicts[j].Facts.Name
	})
	return verdicts
}

// IsProtected reports whether the engine may skip gathering relation facts
// for this branch: protection short-circuits every other rule.
func IsProtected(f Facts, opt Options) bool {
	if f.Name == opt.BaseName || f.IsHead || f.WorktreePath != "" {
		return true
	}
	_, ok := matchProtect(f.Name, opt.Protect)
	return ok
}

// Summary counts verdicts per category plus the deletable total.
type Summary struct {
	Total     int
	PerCat    map[Category]int
	Deletable int
}

// Summarize tallies a verdict list.
func Summarize(verdicts []Verdict) Summary {
	s := Summary{Total: len(verdicts), PerCat: make(map[Category]int)}
	for _, v := range verdicts {
		s.PerCat[v.Category]++
		if v.Deletable {
			s.Deletable++
		}
	}
	return s
}

func protect(v Verdict, reason string) Verdict {
	v.Category = Protected
	v.Reasons = append(v.Reasons, reason)
	return v
}

// matchProtect matches a branch name against --protect patterns. Patterns
// use path.Match semantics ('*' does not cross '/'), and a pattern with no
// metacharacters is an exact name match. Malformed patterns never match.
func matchProtect(name string, patterns []string) (string, bool) {
	for _, pat := range patterns {
		if pat == name {
			return pat, true
		}
		if ok, err := path.Match(pat, name); err == nil && ok {
			return pat, true
		}
	}
	return "", false
}

// secondary lists extra findings worth showing on an already-deletable
// branch, e.g. that its upstream is gone too.
func secondary(f Facts, opt Options) []string {
	var extra []string
	if f.UpstreamGone {
		extra = append(extra, fmt.Sprintf("upstream %s was deleted on the remote", f.Upstream))
	}
	if f.Ahead > 0 {
		extra = append(extra, fmt.Sprintf("ahead %d / behind %d vs %s", f.Ahead, f.Behind, opt.BaseName))
	}
	return extra
}

// relation describes where the branch stands vs base, for evidence lines.
func relation(f Facts, opt Options) string {
	if f.NoCommonAncestor {
		return fmt.Sprintf("no common ancestor with %s", opt.BaseName)
	}
	return fmt.Sprintf("ahead %d / behind %d vs %s", f.Ahead, f.Behind, opt.BaseName)
}

func ageDays(when, now time.Time) int {
	if when.IsZero() {
		return 0
	}
	d := now.Sub(when)
	if d < 0 {
		return 0
	}
	return int(d.Hours() / 24)
}
