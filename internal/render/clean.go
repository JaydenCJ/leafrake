package render

import (
	"fmt"
	"strings"

	"github.com/JaydenCJ/leafrake/internal/classify"
	"github.com/JaydenCJ/leafrake/internal/engine"
)

// CleanPlan renders the dry-run view of a clean: what would be deleted and
// the evidence for each branch, without touching anything.
func CleanPlan(selected []classify.Verdict, categories []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "leafrake clean — dry run (nothing deleted; pass --yes to delete)\n")
	fmt.Fprintf(&b, "selection: %s\n\n", strings.Join(categories, ", "))
	if len(selected) == 0 {
		b.WriteString("no branch matches the selection — nothing to delete\n")
		return b.String()
	}
	nameW := maxNameWidth(selected)
	for _, v := range selected {
		fmt.Fprintf(&b, "would delete  %-*s  %-13s  was %.7s\n",
			nameW, v.Facts.Name, v.Category, v.Facts.Tip)
		for _, reason := range v.Reasons {
			fmt.Fprintf(&b, "  └─ %s\n", reason)
		}
	}
	fmt.Fprintf(&b, "\n%d %s would be deleted — re-run with --yes\n",
		len(selected), plural(len(selected), "branch", "branches"))
	return b.String()
}

// CleanResults renders the outcome of an actual deletion pass, including a
// restore line per deleted branch (the tip hash keeps the commits alive
// until git prunes unreachable objects).
func CleanResults(results []engine.DeleteResult, categories []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "leafrake clean — selection: %s\n\n", strings.Join(categories, ", "))
	if len(results) == 0 {
		b.WriteString("no branch matches the selection — nothing to delete\n")
		return b.String()
	}
	nameW := 0
	for _, r := range results {
		if len(r.Branch) > nameW {
			nameW = len(r.Branch)
		}
	}
	deleted, failed := 0, 0
	for _, r := range results {
		if r.Err != nil {
			failed++
			fmt.Fprintf(&b, "FAILED        %-*s  %-13s  %v\n", nameW, r.Branch, r.Category, r.Err)
			continue
		}
		deleted++
		fmt.Fprintf(&b, "deleted       %-*s  %-13s  was %.7s\n", nameW, r.Branch, r.Category, r.Tip)
		fmt.Fprintf(&b, "  └─ restore: git branch %s %.7s\n", r.Branch, r.Tip)
	}
	fmt.Fprintf(&b, "\n%d deleted, %d failed\n", deleted, failed)
	return b.String()
}

func maxNameWidth(verdicts []classify.Verdict) int {
	w := 0
	for _, v := range verdicts {
		if len(v.Facts.Name) > w {
			w = len(v.Facts.Name)
		}
	}
	return w
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
