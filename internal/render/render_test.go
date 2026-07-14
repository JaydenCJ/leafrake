// Unit tests for the renderers: text layout, JSON envelope stability, the
// clean plan/results views, and the explain dossier — all over hand-built
// reports, so every byte is deterministic.
package render

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/JaydenCJ/leafrake/internal/baseref"
	"github.com/JaydenCJ/leafrake/internal/classify"
	"github.com/JaydenCJ/leafrake/internal/engine"
)

var when = time.Date(2026, 7, 2, 9, 0, 0, 0, time.UTC)

func sampleReport() *engine.Report {
	mergedFacts := classify.Facts{
		Name: "feature/login", Tip: strings.Repeat("1", 40),
		CommitterDate: when, Subject: "Login flow", IsAncestorOfBase: true,
	}
	squashFacts := classify.Facts{
		Name: "feature/search", Tip: strings.Repeat("2", 40),
		CommitterDate: when, Subject: "Search tweaks",
		MergeBase: strings.Repeat("3", 40),
		PatchID:   strings.Repeat("4", 40), Ahead: 3, Behind: 5,
		Upstream: "origin/feature/search",
	}
	activeFacts := classify.Facts{
		Name: "wip", Tip: strings.Repeat("5", 40),
		CommitterDate: when, Subject: "WIP", Ahead: 1, Behind: 0,
	}
	baseFacts := classify.Facts{
		Name: "main", Tip: strings.Repeat("6", 40), IsHead: true,
		CommitterDate: when, Subject: "Squash!",
	}
	idx := classify.BaseIndex{squashFacts.PatchID: {
		Hash: strings.Repeat("6", 40), Subject: "Add search (#42)", When: when,
	}}
	opt := classify.Options{
		BaseName: "main", StaleDays: 90,
		Now: time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC),
	}
	verdicts := classify.DecideAll(
		[]classify.Facts{mergedFacts, squashFacts, activeFacts, baseFacts}, idx, opt)
	return &engine.Report{
		RepoName: "demo", RepoTop: "/work/demo", HeadBranch: "main",
		Base: "main", BaseSource: baseref.SourceOriginHead,
		Verdicts: verdicts, Summary: classify.Summarize(verdicts),
	}
}

func TestTextHeaderAndSummary(t *testing.T) {
	out := Text(sampleReport())
	if !strings.HasPrefix(out, "leafrake scan — demo @ main (base: main, from origin/HEAD)\n") {
		t.Fatalf("header:\n%s", out)
	}
	if !strings.Contains(out, "4 local branches: 1 merged, 1 squash-merged, 1 active, 1 protected") {
		t.Fatalf("summary line:\n%s", out)
	}
}

func TestTextShowsEvidencePerBranch(t *testing.T) {
	out := Text(sampleReport())
	for _, want := range []string{
		"MERGED         feature/login",
		"└─ tip 1111111 is an ancestor of main",
		"SQUASH-MERGED  feature/search",
		`└─ matches squash commit 6666666 "Add search (#42)" (2026-07-02) on main`,
		"deletable now: 2 (merged + squash-merged)",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}

func TestTextNothingDeletable(t *testing.T) {
	r := sampleReport()
	r.Verdicts = r.Verdicts[2:] // keep only active + protected
	r.Summary = classify.Summarize(r.Verdicts)
	out := Text(r)
	if !strings.Contains(out, "nothing is provably dead") {
		t.Fatalf("empty case:\n%s", out)
	}
}

func TestTextDetachedHead(t *testing.T) {
	r := sampleReport()
	r.HeadBranch = ""
	if !strings.Contains(Text(r), "@ (detached HEAD)") {
		t.Fatal("detached HEAD not rendered")
	}
}

func TestJSONEnvelopeShape(t *testing.T) {
	raw, err := JSON(sampleReport())
	if err != nil {
		t.Fatal(err)
	}
	var env struct {
		Tool          string `json:"tool"`
		SchemaVersion int    `json:"schema_version"`
		Base          struct{ Name, Source string }
		Summary       struct {
			Total        int `json:"total"`
			Merged       int `json:"merged"`
			SquashMerged int `json:"squash_merged"`
			DeletableNow int `json:"deletable_now"`
		} `json:"summary"`
		Branches []struct {
			Name        string                            `json:"name"`
			Category    string                            `json:"category"`
			Deletable   bool                              `json:"deletable"`
			SquashMatch *struct{ Commit, Subject string } `json:"squash_match"`
			Evidence    []string                          `json:"evidence"`
		} `json:"branches"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, raw)
	}
	if env.Tool != "leafrake" || env.SchemaVersion != 1 {
		t.Fatalf("envelope: %+v", env)
	}
	if env.Summary.Total != 4 || env.Summary.Merged != 1 ||
		env.Summary.SquashMerged != 1 || env.Summary.DeletableNow != 2 {
		t.Fatalf("summary: %+v", env.Summary)
	}
	if len(env.Branches) != 4 || env.Branches[0].Name != "feature/login" {
		t.Fatalf("branch order: %+v", env.Branches)
	}
	squash := env.Branches[1]
	if squash.Category != "squash-merged" || squash.SquashMatch == nil ||
		squash.SquashMatch.Subject != "Add search (#42)" {
		t.Fatalf("squash proof in JSON: %+v", squash)
	}
	if len(squash.Evidence) == 0 {
		t.Fatal("evidence array must not be empty")
	}
}

func TestJSONDatesAreUTCRFC3339(t *testing.T) {
	raw, err := JSON(sampleReport())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"last_commit": "2026-07-02T09:00:00Z"`) {
		t.Fatalf("dates must be UTC RFC3339:\n%s", raw)
	}
}

func TestCleanPlanListsSelections(t *testing.T) {
	r := sampleReport()
	var selected []classify.Verdict
	for _, v := range r.Verdicts {
		if v.Deletable {
			selected = append(selected, v)
		}
	}
	out := CleanPlan(selected, []string{"merged", "squash-merged"})
	for _, want := range []string{
		"dry run (nothing deleted; pass --yes to delete)",
		"would delete  feature/login",
		"would delete  feature/search",
		"2 branches would be deleted — re-run with --yes",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}

func TestCleanPlanEmptySelection(t *testing.T) {
	out := CleanPlan(nil, []string{"merged", "squash-merged"})
	if !strings.Contains(out, "no branch matches the selection") {
		t.Fatalf("empty plan:\n%s", out)
	}
}

func TestCleanResultsRestoreLinesAndFailures(t *testing.T) {
	results := []engine.DeleteResult{
		{Branch: "feature/login", Tip: strings.Repeat("1", 40), Category: classify.Merged},
		{Branch: "feature/search", Tip: strings.Repeat("2", 40),
			Category: classify.Squashed, Err: errors.New("branch moved")},
	}
	out := CleanResults(results, []string{"merged", "squash-merged"})
	for _, want := range []string{
		"deleted       feature/login",
		"restore: git branch feature/login 1111111",
		"FAILED        feature/search",
		"branch moved",
		"1 deleted, 1 failed",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}

func TestExplainDossier(t *testing.T) {
	r := sampleReport()
	var squash classify.Verdict
	for _, v := range r.Verdicts {
		if v.Category == classify.Squashed {
			squash = v
		}
	}
	out := Explain(r, squash)
	for _, want := range []string{
		"leafrake explain — feature/search",
		"merge-base:    3333333",
		"ahead/behind:  3 / 5 vs main",
		"squash patch-id: " + strings.Repeat("4", 40),
		"verdict: SQUASH-MERGED (deletable)",
		"1. diff vs merge-base",
		"restore later: git branch feature/search 2222222",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}

func TestExplainUpstreamStates(t *testing.T) {
	r := sampleReport()
	cases := []struct {
		facts classify.Facts
		want  string
	}{
		{classify.Facts{Name: "a", CommitterDate: when}, "none configured"},
		{classify.Facts{Name: "b", CommitterDate: when,
			Upstream: "origin/b", UpstreamGone: true}, "origin/b [gone — deleted on the remote]"},
		{classify.Facts{Name: "c", CommitterDate: when,
			Upstream: "origin/c", Track: "[ahead 2]"}, "origin/c [ahead 2]"},
		{classify.Facts{Name: "d", CommitterDate: when,
			Upstream: "origin/d"}, "origin/d [in sync]"},
	}
	for _, c := range cases {
		out := Explain(r, classify.Verdict{Facts: c.facts, Category: classify.Active,
			Reasons: []string{"r"}})
		if !strings.Contains(out, "upstream:      "+c.want) {
			t.Fatalf("upstream %q: missing %q in:\n%s", c.facts.Name, c.want, out)
		}
	}
}
