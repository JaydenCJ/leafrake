// Package engine orchestrates the scan: it gathers repository facts through
// gitio, feeds them to the pure classify rules, and assembles the Report
// that every subcommand renders or acts on.
package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/JaydenCJ/leafrake/internal/baseref"
	"github.com/JaydenCJ/leafrake/internal/classify"
	"github.com/JaydenCJ/leafrake/internal/gitio"
)

// Options configures a scan.
type Options struct {
	Path         string    // repository path, "" = current directory
	Base         string    // --base override, "" = auto-detect
	StaleDays    int       // staleness threshold in days; 0 disables the rule
	SquashWindow int       // base commits to index; <=0 = DefaultSquashWindow
	Protect      []string  // extra protected patterns
	Now          time.Time // injected clock; zero = time.Now()
}

// DefaultStaleDays is the zero-configuration staleness threshold.
const DefaultStaleDays = 90

// DefaultSquashWindow bounds the base-branch patch-id index. 1000 squash
// commits of history is months of work on busy repos; older branches are
// still caught by the ancestor and stale rules.
const DefaultSquashWindow = 1000

// Report is the full result of one scan.
type Report struct {
	RepoName   string // basename of the working-tree root
	RepoTop    string // absolute working-tree root
	HeadBranch string // "" when HEAD is detached
	Base       string
	BaseSource baseref.Source
	Verdicts   []classify.Verdict
	Summary    classify.Summary
}

// Scan classifies every local branch of the repository at opt.Path.
func Scan(opt Options) (*Report, error) {
	if opt.SquashWindow <= 0 {
		opt.SquashWindow = DefaultSquashWindow
	}
	if opt.Now.IsZero() {
		opt.Now = time.Now()
	}
	g := gitio.Git{Dir: opt.Path}

	top, err := g.RepoTop()
	if err != nil {
		// A path that does not exist deserves a direct answer, not git's
		// chdir error — it is usually a typo (path or subcommand).
		dir := opt.Path
		if dir == "" {
			dir = "."
		}
		if _, statErr := os.Stat(dir); statErr != nil {
			return nil, fmt.Errorf("path %q does not exist", dir)
		}
		return nil, fmt.Errorf("not a git repository: %v", err)
	}
	refsRaw, err := g.RefsRaw()
	if err != nil {
		return nil, err
	}
	refs, err := gitio.ParseRefs(refsRaw)
	if err != nil {
		return nil, err
	}
	if len(refs) == 0 {
		return nil, fmt.Errorf("no local branches found (empty repository?)")
	}

	originHead, err := g.OriginHead()
	if err != nil {
		return nil, err
	}
	base, baseSource, err := baseref.Pick(opt.Base, originHead, gitio.Names(refs))
	if err != nil {
		return nil, err
	}
	if _, ok, err := g.ResolveRef(base); err != nil {
		return nil, err
	} else if !ok {
		return nil, fmt.Errorf("base %q does not resolve to a commit", base)
	}

	wtRaw, err := g.WorktreesRaw()
	if err != nil {
		return nil, err
	}
	branchWorktrees := gitio.BranchWorktrees(gitio.ParseWorktrees(wtRaw), top)

	copt := classify.Options{
		BaseName:  base,
		StaleDays: opt.StaleDays,
		Protect:   opt.Protect,
		Now:       opt.Now,
	}

	facts, needIndex, err := gatherFacts(g, refs, branchWorktrees, copt)
	if err != nil {
		return nil, err
	}

	// Build the squash index only when some branch actually needs a patch-id
	// lookup; a repo where everything is merged or protected skips the pass.
	idx := classify.BaseIndex{}
	if needIndex {
		idx, err = buildSquashIndex(g, base, opt.SquashWindow)
		if err != nil {
			return nil, err
		}
	}

	verdicts := classify.DecideAll(facts, idx, copt)
	return &Report{
		RepoName:   filepath.Base(top),
		RepoTop:    top,
		HeadBranch: gitio.HeadBranch(refs),
		Base:       base,
		BaseSource: baseSource,
		Verdicts:   verdicts,
		Summary:    classify.Summarize(verdicts),
	}, nil
}

// gatherFacts collects relation facts (merge-base, ancestry, patch-id,
// ahead/behind) for every branch that protection does not short-circuit.
// needIndex reports whether at least one branch has a patch-id to look up.
func gatherFacts(g gitio.Git, refs []gitio.Ref, worktrees map[string]string,
	copt classify.Options) (facts []classify.Facts, needIndex bool, err error) {

	for _, r := range refs {
		f := classify.Facts{
			Name:          r.Name,
			Tip:           r.Hash,
			IsHead:        r.IsHead,
			WorktreePath:  worktrees[r.Name],
			Upstream:      r.Upstream,
			UpstreamGone:  r.UpstreamGone,
			Track:         r.Track,
			CommitterDate: r.CommitterDate,
			Subject:       r.Subject,
		}
		if classify.IsProtected(f, copt) {
			facts = append(facts, f)
			continue
		}
		if err := relateToBase(g, copt.BaseName, &f); err != nil {
			return nil, false, fmt.Errorf("branch %s: %v", r.Name, err)
		}
		if f.PatchID != "" {
			needIndex = true
		}
		facts = append(facts, f)
	}
	return facts, needIndex, nil
}

// relateToBase fills the merge-base, ancestry, squash patch-id, and
// ahead/behind fields of one branch's facts.
func relateToBase(g gitio.Git, base string, f *classify.Facts) error {
	mergeBase, ok, err := g.MergeBase(base, f.Tip)
	if err != nil {
		return err
	}
	if !ok {
		f.NoCommonAncestor = true
		return nil
	}
	f.MergeBase = mergeBase

	ancestor, err := g.IsAncestor(f.Tip, base)
	if err != nil {
		return err
	}
	f.IsAncestorOfBase = ancestor

	ahead, behind, err := g.AheadBehind(base, f.Tip)
	if err != nil {
		return err
	}
	f.Ahead, f.Behind = ahead, behind

	if ancestor {
		return nil // proof complete; no patch-id needed
	}
	patchID, err := g.DiffPatchID(mergeBase, f.Tip)
	if err != nil {
		return err
	}
	if patchID == "" {
		f.EmptyDiff = true
		return nil
	}
	f.PatchID = patchID
	return nil
}

// buildSquashIndex maps patch-id → base commit for the last `window`
// first-parent commits of base, joining the patch-id stream with the
// hash/subject/date log so matches carry quotable evidence.
func buildSquashIndex(g gitio.Git, base string, window int) (classify.BaseIndex, error) {
	pidsRaw, err := g.BasePatchIDsRaw(base, window)
	if err != nil {
		return nil, err
	}
	metaRaw, err := g.BaseLogRaw(base, window)
	if err != nil {
		return nil, err
	}
	meta, err := gitio.ParseBaseLog(metaRaw)
	if err != nil {
		return nil, err
	}
	byHash := make(map[string]gitio.BaseCommit, len(meta))
	for _, c := range meta {
		byHash[c.Hash] = c
	}
	idx := make(classify.BaseIndex)
	for _, e := range gitio.ParsePatchIDs(pidsRaw) {
		if _, dup := idx[e.PatchID]; dup {
			continue // keep the newest commit (log is newest-first)
		}
		m := classify.SquashMatch{Hash: e.Commit}
		if c, ok := byHash[e.Commit]; ok {
			m.Subject = c.Subject
			m.When = c.When
		}
		idx[e.PatchID] = m
	}
	return idx, nil
}

// DeleteResult records the outcome of one attempted branch deletion.
type DeleteResult struct {
	Branch   string
	Tip      string
	Category classify.Category
	Err      error // nil on success
}

// Delete removes the given verdicts' branches, re-verifying each tip hash
// immediately before deletion. It keeps going after individual failures so
// one moved branch does not abort the rest of the cleanup.
func Delete(path string, verdicts []classify.Verdict) []DeleteResult {
	g := gitio.Git{Dir: path}
	results := make([]DeleteResult, 0, len(verdicts))
	for _, v := range verdicts {
		res := DeleteResult{
			Branch:   v.Facts.Name,
			Tip:      v.Facts.Tip,
			Category: v.Category,
		}
		res.Err = g.DeleteBranch(v.Facts.Name, v.Facts.Tip)
		results = append(results, res)
	}
	return results
}
