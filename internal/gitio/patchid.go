package gitio

import "strings"

// PatchEntry is one line of `git patch-id --stable` output: the stable
// patch-id of a commit's diff, plus the commit hash it was computed from
// (all-zero when the input was a bare diff with no commit header).
type PatchEntry struct {
	PatchID string
	Commit  string
}

// ParsePatchIDs parses `git patch-id --stable` output. Lines that do not
// have exactly two fields are skipped defensively; git never emits them.
func ParsePatchIDs(raw []byte) []PatchEntry {
	var entries []PatchEntry
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		entries = append(entries, PatchEntry{PatchID: fields[0], Commit: fields[1]})
	}
	return entries
}

// FirstPatchID returns the patch-id of the first entry, or "" when the
// input contains none (an empty diff produces no output at all).
func FirstPatchID(raw []byte) string {
	entries := ParsePatchIDs(raw)
	if len(entries) == 0 {
		return ""
	}
	return entries[0].PatchID
}
