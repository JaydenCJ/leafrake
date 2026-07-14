// Unit tests for the pure classification rules: protection precedence,
// merge proofs (ancestor, empty diff, squash patch-id), gone and stale
// fallbacks, and the deterministic verdict ordering.
package classify

import (
	"strings"
	"testing"
	"time"
)

// now is the injected clock for every test: rules must never read the wall
// clock themselves.
var now = time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)

func opts() Options {
	return Options{BaseName: "main", StaleDays: 90, Now: now}
}

// branch builds baseline facts for a branch last touched `age` days ago,
// unmerged and unrelated to any proof.
func branch(name string, age int) Facts {
	return Facts{
		Name:          name,
		Tip:           strings.Repeat("a", 40),
		CommitterDate: now.AddDate(0, 0, -age),
		Subject:       "some work",
		MergeBase:     strings.Repeat("b", 40),
		Ahead:         2,
		Behind:        3,
	}
}

func TestBaseBranchIsProtected(t *testing.T) {
	v := Decide(branch("main", 1), nil, opts())
	if v.Category != Protected || v.Deletable {
		t.Fatalf("base branch: %+v", v)
	}
	if v.Reasons[0] != "this is the base branch" {
		t.Fatalf("reason: %q", v.Reasons[0])
	}
}

func TestHeadBranchIsProtected(t *testing.T) {
	f := branch("feature", 1)
	f.IsHead = true
	f.IsAncestorOfBase = true // even a provably merged HEAD stays protected
	v := Decide(f, nil, opts())
	if v.Category != Protected || v.Deletable {
		t.Fatalf("HEAD branch: %+v", v)
	}
}

func TestWorktreeBranchIsProtected(t *testing.T) {
	f := branch("feature", 1)
	f.WorktreePath = "/somewhere/wt"
	v := Decide(f, nil, opts())
	if v.Category != Protected {
		t.Fatalf("worktree branch: %+v", v)
	}
	if !strings.Contains(v.Reasons[0], "/somewhere/wt") {
		t.Fatalf("worktree path missing from evidence: %q", v.Reasons[0])
	}
}

func TestProtectGlobMatches(t *testing.T) {
	o := opts()
	o.Protect = []string{"release/*"}
	v := Decide(branch("release/v1.2", 1), nil, o)
	if v.Category != Protected {
		t.Fatalf("release/v1.2 should match release/*: %+v", v)
	}
}

func TestProtectGlobStarDoesNotCrossSlash(t *testing.T) {
	o := opts()
	o.Protect = []string{"release/*"}
	v := Decide(branch("release/v1/hotfix", 400), nil, o)
	if v.Category == Protected {
		t.Fatal("path.Match '*' must not cross '/'")
	}
}

func TestProtectExactNameNeedsNoGlob(t *testing.T) {
	o := opts()
	o.Protect = []string{"staging"}
	v := Decide(branch("staging", 400), nil, o)
	if v.Category != Protected {
		t.Fatalf("exact protect name: %+v", v)
	}
}

func TestProtectMalformedPatternNeverMatches(t *testing.T) {
	o := opts()
	o.Protect = []string{"[unclosed"}
	v := Decide(branch("unrelated", 1), nil, o)
	if v.Category == Protected {
		t.Fatal("malformed pattern must not protect anything")
	}
}

func TestAncestorOfBaseIsMerged(t *testing.T) {
	f := branch("feature/login", 5)
	f.IsAncestorOfBase = true
	f.Ahead = 0
	v := Decide(f, nil, opts())
	if v.Category != Merged || !v.Deletable {
		t.Fatalf("ancestor: %+v", v)
	}
	if !strings.Contains(v.Reasons[0], "ancestor of main") {
		t.Fatalf("evidence: %q", v.Reasons[0])
	}
}

func TestEmptyDiffIsMerged(t *testing.T) {
	f := branch("net-zero", 5)
	f.EmptyDiff = true
	v := Decide(f, nil, opts())
	if v.Category != Merged || !v.Deletable {
		t.Fatalf("empty diff: %+v", v)
	}
	if !strings.Contains(v.Reasons[0], "net-zero diff") {
		t.Fatalf("evidence: %q", v.Reasons[0])
	}
}

func TestSquashPatchIDMatchIsSquashMerged(t *testing.T) {
	f := branch("feature/search", 5)
	f.PatchID = "17174ad41fd36dc54a3c1ecd78f695b4895e96b5"
	idx := BaseIndex{f.PatchID: {
		Hash:    "96af8e50431e99b7c27b01597bb08aad51b09936",
		Subject: "Add search (#42)",
		When:    time.Date(2026, 7, 2, 9, 0, 0, 0, time.UTC),
	}}
	v := Decide(f, idx, opts())
	if v.Category != Squashed || !v.Deletable {
		t.Fatalf("squash: %+v", v)
	}
	if v.Squash == nil || v.Squash.Hash != "96af8e50431e99b7c27b01597bb08aad51b09936" {
		t.Fatalf("squash proof missing: %+v", v.Squash)
	}
	joined := strings.Join(v.Reasons, "\n")
	if !strings.Contains(joined, "96af8e5") || !strings.Contains(joined, "Add search (#42)") {
		t.Fatalf("evidence must quote the squash commit:\n%s", joined)
	}
}

func TestPatchIDWithoutMatchIsNotSquashed(t *testing.T) {
	f := branch("feature/other", 5)
	f.PatchID = strings.Repeat("d", 40)
	v := Decide(f, BaseIndex{}, opts())
	if v.Category == Squashed || v.Deletable {
		t.Fatalf("no index hit must not delete: %+v", v)
	}
}

func TestGoneUpstreamUnproven(t *testing.T) {
	f := branch("fix/typo", 5)
	f.Upstream = "origin/fix/typo"
	f.UpstreamGone = true
	v := Decide(f, nil, opts())
	if v.Category != Gone || v.Deletable {
		t.Fatalf("gone: %+v", v)
	}
	joined := strings.Join(v.Reasons, "\n")
	if !strings.Contains(joined, "origin/fix/typo") || !strings.Contains(joined, "not proven merged") {
		t.Fatalf("gone evidence:\n%s", joined)
	}
}

func TestGoneOutranksStale(t *testing.T) {
	f := branch("fix/old", 400)
	f.Upstream = "origin/fix/old"
	f.UpstreamGone = true
	v := Decide(f, nil, opts())
	if v.Category != Gone {
		t.Fatalf("gone should outrank stale: %+v", v)
	}
}

func TestMergedBranchWithGoneUpstreamStaysMergedWithExtraEvidence(t *testing.T) {
	// The common post-PR shape: squash-merged AND remote branch deleted.
	// The proof category wins; the gone state appears as evidence.
	f := branch("feature/done", 5)
	f.IsAncestorOfBase = true
	f.Ahead = 0
	f.Upstream = "origin/feature/done"
	f.UpstreamGone = true
	v := Decide(f, nil, opts())
	if v.Category != Merged {
		t.Fatalf("proof must win over gone: %+v", v)
	}
	if !strings.Contains(strings.Join(v.Reasons, "\n"), "deleted on the remote") {
		t.Fatalf("gone evidence missing:\n%v", v.Reasons)
	}
}

func TestStaleAtExactThreshold(t *testing.T) {
	v := Decide(branch("spike/old", 90), nil, opts())
	if v.Category != Stale {
		t.Fatalf("exactly 90 days old must be stale: %+v", v)
	}
}

func TestFreshBranchIsActive(t *testing.T) {
	v := Decide(branch("feature/wip", 89), nil, opts())
	if v.Category != Active || v.Deletable {
		t.Fatalf("89 days old must be active: %+v", v)
	}
	if !strings.Contains(v.Reasons[0], "ahead 2 / behind 3 vs main") {
		t.Fatalf("active evidence: %q", v.Reasons[0])
	}
}

func TestStaleDaysZeroDisablesStaleness(t *testing.T) {
	o := opts()
	o.StaleDays = 0
	v := Decide(branch("ancient", 5000), nil, o)
	if v.Category != Active {
		t.Fatalf("stale-days 0 must disable the rule: %+v", v)
	}
}

func TestNoCommonAncestorIsNeverDeletable(t *testing.T) {
	f := branch("orphan", 5)
	f.MergeBase = ""
	f.NoCommonAncestor = true
	v := Decide(f, nil, opts())
	if v.Deletable {
		t.Fatalf("unrelated history must not be deletable: %+v", v)
	}
	if !strings.Contains(strings.Join(v.Reasons, "\n"), "no common ancestor") {
		t.Fatalf("evidence: %v", v.Reasons)
	}
}

func TestFutureCommitDateClampsToZeroAge(t *testing.T) {
	// Clock skew: a commit "from the future" must not underflow into stale.
	v := Decide(branch("time-traveler", -5), nil, opts())
	if v.Category != Active {
		t.Fatalf("future-dated commit: %+v", v)
	}
	if !strings.Contains(v.Reasons[0], "0 days ago") {
		t.Fatalf("age must clamp to 0: %q", v.Reasons[0])
	}
}

func TestDecideAllSortsByCategoryThenName(t *testing.T) {
	merged := branch("zzz-merged", 5)
	merged.IsAncestorOfBase = true
	stale := branch("aaa-stale", 400)
	active := branch("mmm-active", 1)
	base := branch("main", 1)
	verdicts := DecideAll([]Facts{stale, base, active, merged}, nil, opts())
	var order []string
	for _, v := range verdicts {
		order = append(order, v.Facts.Name)
	}
	want := "zzz-merged,aaa-stale,mmm-active,main"
	if got := strings.Join(order, ","); got != want {
		t.Fatalf("order: want %s, got %s", want, got)
	}
}

func TestSummarizeCounts(t *testing.T) {
	merged := branch("m", 5)
	merged.IsAncestorOfBase = true
	stale := branch("s", 400)
	verdicts := DecideAll([]Facts{merged, stale, branch("main", 1)}, nil, opts())
	sum := Summarize(verdicts)
	if sum.Total != 3 || sum.Deletable != 1 {
		t.Fatalf("summary: %+v", sum)
	}
	if sum.PerCat[Merged] != 1 || sum.PerCat[Stale] != 1 || sum.PerCat[Protected] != 1 {
		t.Fatalf("per-category: %+v", sum.PerCat)
	}
}

func TestIsProtectedShortCircuit(t *testing.T) {
	o := opts()
	o.Protect = []string{"keep-*"}
	head := branch("x", 1)
	head.IsHead = true
	for _, f := range []Facts{branch("main", 1), head, branch("keep-me", 1)} {
		if !IsProtected(f, o) {
			t.Fatalf("%s should be protected", f.Name)
		}
	}
	if IsProtected(branch("normal", 1), o) {
		t.Fatal("normal branch must not be protected")
	}
}
