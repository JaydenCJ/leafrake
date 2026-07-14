// Unit tests for the for-each-ref parser: field mapping, HEAD and [gone]
// markers, odd-but-legal branch names, and malformed input rejection.
package gitio

import (
	"strings"
	"testing"
	"time"
)

// line builds one refFormat record from its seven fields.
func line(fields ...string) string {
	return strings.Join(fields, "\x1f")
}

func TestParseRefsSingleBranchAllFields(t *testing.T) {
	raw := line("*", "main", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"origin/main", "", "2026-03-01T10:00:00+00:00", "Initial commit") + "\n"
	refs, err := ParseRefs([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 {
		t.Fatalf("want 1 ref, got %d", len(refs))
	}
	r := refs[0]
	if !r.IsHead || r.Name != "main" || r.Upstream != "origin/main" ||
		r.UpstreamGone || r.Subject != "Initial commit" {
		t.Fatalf("bad parse: %+v", r)
	}
	want := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	if !r.CommitterDate.Equal(want) {
		t.Fatalf("date: want %v, got %v", want, r.CommitterDate)
	}
}

func TestParseRefsGoneMarker(t *testing.T) {
	raw := line(" ", "fix/typo", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		"origin/fix/typo", "[gone]", "2026-03-02T10:00:00+00:00", "Fix typo") + "\n"
	refs, err := ParseRefs([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if !refs[0].UpstreamGone {
		t.Fatal("want UpstreamGone=true for [gone] track")
	}
	if refs[0].Track != "[gone]" {
		t.Fatalf("raw track: got %q", refs[0].Track)
	}
}

func TestParseRefsAheadTrackIsNotGone(t *testing.T) {
	raw := line(" ", "feature", "cccccccccccccccccccccccccccccccccccccccc",
		"origin/feature", "[ahead 2, behind 1]", "2026-03-02T10:00:00+00:00", "WIP") + "\n"
	refs, err := ParseRefs([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if refs[0].UpstreamGone {
		t.Fatal("[ahead …] must not be treated as gone")
	}
}

func TestParseRefsNoUpstream(t *testing.T) {
	raw := line(" ", "local-only", "dddddddddddddddddddddddddddddddddddddddd",
		"", "", "2026-03-03T10:00:00+00:00", "Local work") + "\n"
	refs, err := ParseRefs([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if refs[0].Upstream != "" || refs[0].UpstreamGone {
		t.Fatalf("want no upstream: %+v", refs[0])
	}
}

func TestParseRefsSlashesAndDotsInName(t *testing.T) {
	// Branch names may contain slashes, dots, and unicode.
	raw := line(" ", "release/v1.2.x", "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
		"", "", "2026-03-04T10:00:00+00:00", "Release prep") + "\n"
	refs, err := ParseRefs([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if refs[0].Name != "release/v1.2.x" {
		t.Fatalf("name: got %q", refs[0].Name)
	}
}

func TestParseRefsMultipleBranchesPreserveOrder(t *testing.T) {
	raw := line(" ", "alpha", strings.Repeat("a", 40), "", "", "2026-01-01T00:00:00+00:00", "a") + "\n" +
		line("*", "beta", strings.Repeat("b", 40), "", "", "2026-01-02T00:00:00+00:00", "b") + "\n" +
		line(" ", "gamma", strings.Repeat("c", 40), "", "", "2026-01-03T00:00:00+00:00", "c") + "\n"
	refs, err := ParseRefs([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if got := Names(refs); got[0] != "alpha" || got[1] != "beta" || got[2] != "gamma" {
		t.Fatalf("order not preserved: %v", got)
	}
	if HeadBranch(refs) != "beta" {
		t.Fatalf("HeadBranch: got %q", HeadBranch(refs))
	}
}

func TestParseRefsEmptyInput(t *testing.T) {
	refs, err := ParseRefs(nil)
	if err != nil || len(refs) != 0 {
		t.Fatalf("empty input: refs=%v err=%v", refs, err)
	}
}

func TestParseRefsSubjectMayContainSeparatorLookalikes(t *testing.T) {
	// Subjects with pipes, tabs, and quotes must survive verbatim; only the
	// unit separator splits fields.
	subject := `fix: handle "a | b" and	tabs`
	raw := line(" ", "x", strings.Repeat("f", 40), "", "", "2026-01-01T00:00:00+00:00", subject) + "\n"
	refs, err := ParseRefs([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if refs[0].Subject != subject {
		t.Fatalf("subject mangled: %q", refs[0].Subject)
	}
}

func TestParseRefsRejectsWrongFieldCount(t *testing.T) {
	if _, err := ParseRefs([]byte("just-one-field\n")); err == nil {
		t.Fatal("want error for wrong field count")
	}
}

func TestParseRefsRejectsBadDate(t *testing.T) {
	raw := line(" ", "x", strings.Repeat("a", 40), "", "", "not-a-date", "subj") + "\n"
	if _, err := ParseRefs([]byte(raw)); err == nil {
		t.Fatal("want error for unparseable date")
	}
}

func TestHeadBranchDetachedReturnsEmpty(t *testing.T) {
	raw := line(" ", "main", strings.Repeat("a", 40), "", "", "2026-01-01T00:00:00+00:00", "a") + "\n"
	refs, err := ParseRefs([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if HeadBranch(refs) != "" {
		t.Fatal("detached HEAD (no starred ref) must yield empty head branch")
	}
}
