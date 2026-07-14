package render

import (
	"fmt"
	"strings"

	"github.com/JaydenCJ/leafrake/internal/classify"
	"github.com/JaydenCJ/leafrake/internal/engine"
)

// Explain renders the full evidence dossier for a single branch.
func Explain(r *engine.Report, v classify.Verdict) string {
	f := v.Facts
	var b strings.Builder
	fmt.Fprintf(&b, "leafrake explain — %s\n\n", f.Name)
	fmt.Fprintf(&b, "branch:        %s\n", f.Name)
	fmt.Fprintf(&b, "tip:           %.7s %q (%s)\n",
		f.Tip, f.Subject, f.CommitterDate.Format("2006-01-02"))
	fmt.Fprintf(&b, "upstream:      %s\n", upstreamLine(f))
	fmt.Fprintf(&b, "base:          %s (from %s)\n", r.Base, r.BaseSource)
	if f.NoCommonAncestor {
		fmt.Fprintf(&b, "merge-base:    none (unrelated history)\n")
	} else if f.MergeBase != "" {
		fmt.Fprintf(&b, "merge-base:    %.7s\n", f.MergeBase)
		fmt.Fprintf(&b, "ahead/behind:  %d / %d vs %s\n", f.Ahead, f.Behind, r.Base)
	}
	if f.PatchID != "" {
		fmt.Fprintf(&b, "squash patch-id: %s\n", f.PatchID)
	}
	fmt.Fprintf(&b, "\nverdict: %s%s\n", strings.ToUpper(string(v.Category)), deletableTag(v))
	b.WriteString("evidence:\n")
	for i, reason := range v.Reasons {
		fmt.Fprintf(&b, "  %d. %s\n", i+1, reason)
	}
	if v.Deletable {
		fmt.Fprintf(&b, "\ndelete with: leafrake clean --yes   (restore later: git branch %s %.7s)\n",
			f.Name, f.Tip)
	}
	return b.String()
}

func upstreamLine(f classify.Facts) string {
	if f.Upstream == "" {
		return "none configured"
	}
	if f.UpstreamGone {
		return f.Upstream + " [gone — deleted on the remote]"
	}
	if f.Track != "" {
		return f.Upstream + " " + f.Track
	}
	return f.Upstream + " [in sync]"
}

func deletableTag(v classify.Verdict) string {
	if v.Deletable {
		return " (deletable)"
	}
	return ""
}
