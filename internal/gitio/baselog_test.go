// Unit tests for the base-branch log metadata parser (hash / subject /
// committer date), which turns squash-index hits into quotable evidence.
package gitio

import (
	"strings"
	"testing"
	"time"
)

func logLine(hash, subject, date string) string {
	return hash + "\x1f" + subject + "\x1f" + date
}

func TestParseBaseLogTwoCommits(t *testing.T) {
	raw := logLine(strings.Repeat("a", 40), "Add search (#42)", "2026-07-02T09:00:00+00:00") + "\n" +
		logLine(strings.Repeat("b", 40), "Initial commit", "2026-01-01T00:00:00+00:00") + "\n"
	commits, err := ParseBaseLog([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != 2 {
		t.Fatalf("want 2 commits, got %d", len(commits))
	}
	if commits[0].Subject != "Add search (#42)" {
		t.Fatalf("subject: got %q", commits[0].Subject)
	}
	want := time.Date(2026, 7, 2, 9, 0, 0, 0, time.UTC)
	if !commits[0].When.Equal(want) {
		t.Fatalf("date: want %v, got %v", want, commits[0].When)
	}
}

func TestParseBaseLogEmptyInput(t *testing.T) {
	commits, err := ParseBaseLog(nil)
	if err != nil || len(commits) != 0 {
		t.Fatalf("empty input: commits=%v err=%v", commits, err)
	}
}

func TestParseBaseLogSubjectWithUnicodeAndQuotes(t *testing.T) {
	subject := `feat: 日本語サポート "quoted"`
	raw := logLine(strings.Repeat("c", 40), subject, "2026-05-05T05:05:05+09:00") + "\n"
	commits, err := ParseBaseLog([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if commits[0].Subject != subject {
		t.Fatalf("subject mangled: %q", commits[0].Subject)
	}
}

func TestParseBaseLogRejectsWrongFieldCount(t *testing.T) {
	if _, err := ParseBaseLog([]byte("only two\x1ffields\n")); err == nil {
		t.Fatal("want error for wrong field count")
	}
}

func TestParseBaseLogRejectsBadDate(t *testing.T) {
	raw := logLine(strings.Repeat("d", 40), "subj", "yesterday") + "\n"
	if _, err := ParseBaseLog([]byte(raw)); err == nil {
		t.Fatal("want error for unparseable date")
	}
}
