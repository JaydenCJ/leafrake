// Package render turns scan reports and clean plans into terminal text and
// stable JSON. Everything here is pure formatting over data — no git, no
// clock — so outputs are byte-for-byte reproducible in tests.
package render

import (
	"fmt"
	"strings"

	"github.com/JaydenCJ/leafrake/internal/classify"
	"github.com/JaydenCJ/leafrake/internal/engine"
)

// categoryOrder is the fixed display order for summaries.
var categoryOrder = []classify.Category{
	classify.Merged, classify.Squashed, classify.Gone,
	classify.Stale, classify.Active, classify.Protected,
}

// Text renders the scan report for humans: one block per branch, evidence
// lines indented beneath, deletable categories first.
func Text(r *engine.Report) string {
	var b strings.Builder
	head := r.HeadBranch
	if head == "" {
		head = "(detached HEAD)"
	}
	fmt.Fprintf(&b, "leafrake scan — %s @ %s (base: %s, from %s)\n",
		r.RepoName, head, r.Base, r.BaseSource)
	fmt.Fprintf(&b, "%s\n\n", summaryLine(r.Summary))

	nameW := 0
	for _, v := range r.Verdicts {
		if len(v.Facts.Name) > nameW {
			nameW = len(v.Facts.Name)
		}
	}
	for _, v := range r.Verdicts {
		fmt.Fprintf(&b, "%-13s  %-*s  %.7s\n",
			strings.ToUpper(string(v.Category)), nameW, v.Facts.Name, v.Facts.Tip)
		for _, reason := range v.Reasons {
			fmt.Fprintf(&b, "  └─ %s\n", reason)
		}
	}

	b.WriteString("\n")
	if r.Summary.Deletable > 0 {
		fmt.Fprintf(&b, "deletable now: %d (merged + squash-merged) — run `leafrake clean` to review, `--yes` to delete\n",
			r.Summary.Deletable)
	} else {
		b.WriteString("nothing is provably dead — no branch to delete\n")
	}
	return b.String()
}

// summaryLine renders "8 local branches: 2 merged, 1 squash-merged, …",
// omitting zero-count categories.
func summaryLine(s classify.Summary) string {
	noun := "local branches"
	if s.Total == 1 {
		noun = "local branch"
	}
	var parts []string
	for _, c := range categoryOrder {
		if n := s.PerCat[c]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, c))
		}
	}
	if len(parts) == 0 {
		return fmt.Sprintf("%d %s", s.Total, noun)
	}
	return fmt.Sprintf("%d %s: %s", s.Total, noun, strings.Join(parts, ", "))
}
