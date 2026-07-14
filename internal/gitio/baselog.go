package gitio

import (
	"fmt"
	"strings"
	"time"
)

// BaseCommit is one first-parent commit on the base branch, used to turn a
// matching patch-id into quotable evidence (hash, subject, date).
type BaseCommit struct {
	Hash    string
	Subject string
	When    time.Time
}

// ParseBaseLog parses baseLogFormat output (hash, subject, committer date,
// unit-separator delimited, one line per commit, newest first).
func ParseBaseLog(raw []byte) ([]BaseCommit, error) {
	var commits []BaseCommit
	for _, line := range strings.Split(string(raw), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\x1f")
		if len(fields) != 3 {
			return nil, fmt.Errorf("base log: want 3 fields, got %d in %q", len(fields), line)
		}
		when, err := time.Parse(time.RFC3339, fields[2])
		if err != nil {
			return nil, fmt.Errorf("base log: bad date %q: %v", fields[2], err)
		}
		commits = append(commits, BaseCommit{Hash: fields[0], Subject: fields[1], When: when})
	}
	return commits, nil
}
