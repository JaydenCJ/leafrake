// Unit tests for the `git patch-id --stable` output parser.
package gitio

import "testing"

const (
	pid1  = "17174ad41fd36dc54a3c1ecd78f695b4895e96b5"
	sha1  = "96af8e50431e99b7c27b01597bb08aad51b09936"
	pid2  = "5fbcc3e7625125fe7b462908d15d9b39a26b097e"
	sha2  = "1b2f75ed8d5d16f0252fb2937c8aed0a6b3960bd"
	zeros = "0000000000000000000000000000000000000000"
)

func TestParsePatchIDsTwoCommits(t *testing.T) {
	raw := []byte(pid1 + " " + sha1 + "\n" + pid2 + " " + sha2 + "\n")
	entries := ParsePatchIDs(raw)
	if len(entries) != 2 {
		t.Fatalf("want 2 entries, got %d", len(entries))
	}
	if entries[0].PatchID != pid1 || entries[0].Commit != sha1 {
		t.Fatalf("first entry wrong: %+v", entries[0])
	}
	if entries[1].PatchID != pid2 || entries[1].Commit != sha2 {
		t.Fatalf("second entry wrong: %+v", entries[1])
	}
}

func TestParsePatchIDsBareDiffHasZeroCommit(t *testing.T) {
	// A raw `git diff | git patch-id` line carries the all-zero commit id.
	entries := ParsePatchIDs([]byte(pid1 + " " + zeros + "\n"))
	if len(entries) != 1 || entries[0].Commit != zeros {
		t.Fatalf("bare diff entry wrong: %+v", entries)
	}
}

func TestParsePatchIDsEmptyInput(t *testing.T) {
	if got := ParsePatchIDs(nil); len(got) != 0 {
		t.Fatalf("empty input: got %v", got)
	}
	if got := ParsePatchIDs([]byte("\n\n")); len(got) != 0 {
		t.Fatalf("blank lines: got %v", got)
	}
}

func TestParsePatchIDsSkipsMalformedLines(t *testing.T) {
	raw := []byte("garbage\n" + pid1 + " " + sha1 + "\nthree fields here\n")
	entries := ParsePatchIDs(raw)
	if len(entries) != 1 || entries[0].PatchID != pid1 {
		t.Fatalf("want only the well-formed line: %+v", entries)
	}
}

func TestFirstPatchIDReturnsFirst(t *testing.T) {
	raw := []byte(pid1 + " " + zeros + "\n" + pid2 + " " + zeros + "\n")
	if got := FirstPatchID(raw); got != pid1 {
		t.Fatalf("want %s, got %s", pid1, got)
	}
}

func TestFirstPatchIDEmptyDiffIsEmptyString(t *testing.T) {
	// `git patch-id` on an empty diff prints nothing — that must map to "".
	if got := FirstPatchID(nil); got != "" {
		t.Fatalf("want empty, got %q", got)
	}
}
