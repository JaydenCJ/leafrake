package gitio

import (
	"fmt"
	"strings"
	"time"
)

// Ref is one local branch as reported by `git for-each-ref refs/heads`.
type Ref struct {
	Name          string    // short branch name, e.g. "feature/login"
	Hash          string    // full commit hash of the tip
	IsHead        bool      // checked out in the current worktree
	Upstream      string    // upstream short name, "" when no upstream is set
	UpstreamGone  bool      // upstream configured but deleted on the remote
	Track         string    // raw %(upstream:track), e.g. "[ahead 2]" or "[gone]"
	CommitterDate time.Time // committer date of the tip commit
	Subject       string    // first line of the tip commit message
}

// ParseRefs parses refFormat output (one unit-separator-delimited line per
// branch) into Ref records. Order is preserved as git emitted it.
func ParseRefs(raw []byte) ([]Ref, error) {
	var refs []Ref
	for _, line := range strings.Split(string(raw), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\x1f")
		if len(fields) != 7 {
			return nil, fmt.Errorf("for-each-ref: want 7 fields, got %d in %q", len(fields), line)
		}
		date, err := time.Parse(time.RFC3339, fields[5])
		if err != nil {
			return nil, fmt.Errorf("for-each-ref: bad committer date %q: %v", fields[5], err)
		}
		refs = append(refs, Ref{
			IsHead:        fields[0] == "*",
			Name:          fields[1],
			Hash:          fields[2],
			Upstream:      fields[3],
			Track:         fields[4],
			UpstreamGone:  fields[4] == "[gone]",
			CommitterDate: date,
			Subject:       fields[6],
		})
	}
	return refs, nil
}

// HeadBranch returns the name of the branch checked out in the current
// worktree, or "" when HEAD is detached.
func HeadBranch(refs []Ref) string {
	for _, r := range refs {
		if r.IsHead {
			return r.Name
		}
	}
	return ""
}

// Names returns the branch names in emitted order.
func Names(refs []Ref) []string {
	names := make([]string, len(refs))
	for i, r := range refs {
		names[i] = r.Name
	}
	return names
}
