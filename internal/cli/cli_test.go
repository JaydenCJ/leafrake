// End-to-end tests: build real (temporary, offline) git repositories with
// fixed identities and dates, then run the CLI in-process and assert on its
// stdout, stderr, and exit codes. The "remote" is a bare repository on
// disk, so gone-upstream scenarios need no network. Everything is
// deterministic — commit dates are pinned, git config is isolated from the
// host user.
package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitEnv returns a fully isolated environment for git subprocesses so the
// host machine's config (signing, hooks, templates) cannot leak in.
func gitEnv(dir string, seq int) []string {
	// Old pinned dates (2020) keep the stale fixtures stale forever; recent
	// branches instead pin --stale-days high enough in the assertions.
	date := fmt.Sprintf("2020-01-%02dT10:00:00+00:00", seq)
	return append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=Dev",
		"GIT_AUTHOR_EMAIL=dev@example.test",
		"GIT_COMMITTER_NAME=Dev",
		"GIT_COMMITTER_EMAIL=dev@example.test",
		"GIT_AUTHOR_DATE="+date,
		"GIT_COMMITTER_DATE="+date,
		"HOME="+dir,
	)
}

func mustGit(t *testing.T, dir string, seq int, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = gitEnv(dir, seq)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, errBuf.String())
	}
	return strings.TrimSpace(out.String())
}

func write(t *testing.T, dir, rel, content string) {
	t.Helper()
	abs := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// commit stages everything and commits with a pinned identity and date.
func commit(t *testing.T, dir string, seq int, message string) {
	t.Helper()
	mustGit(t, dir, seq, "add", "-A")
	mustGit(t, dir, seq, "commit", "-q", "--no-gpg-sign", "--allow-empty", "-m", message)
}

// messyRepo builds the canonical fixture: a repo with one branch of every
// category, plus a bare on-disk "origin" for the gone scenario.
//
//	main                 base (holds a squash of feature/search)
//	feature/login        merged into main via merge commit
//	feature/search       squash-merged into main (patch-id proof)
//	fix/typo             upstream deleted on the bare remote → gone
//	spike/old            unmerged, last commit 2020 → stale
//	wip                  unmerged, fresh relative to --stale-days 0 checks
func messyRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "repo")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	mustGit(t, root, 1, "init", "-q", "--bare", "origin.git")
	mustGit(t, dir, 1, "init", "-q")
	mustGit(t, dir, 1, "checkout", "-q", "-b", "main")
	mustGit(t, dir, 1, "remote", "add", "origin", filepath.Join(root, "origin.git"))

	write(t, dir, "app.txt", "v1\n")
	commit(t, dir, 1, "Initial commit")

	// feature/login: two commits, landed with a real merge commit.
	mustGit(t, dir, 2, "checkout", "-q", "-b", "feature/login")
	write(t, dir, "login.txt", "login v1\n")
	commit(t, dir, 2, "Login: skeleton")
	write(t, dir, "login.txt", "login v2\n")
	commit(t, dir, 3, "Login: polish")
	mustGit(t, dir, 4, "checkout", "-q", "main")
	mustGit(t, dir, 4, "merge", "-q", "--no-ff", "--no-edit", "feature/login")

	// feature/search: two commits, squash-merged (no merge parent).
	mustGit(t, dir, 5, "checkout", "-q", "-b", "feature/search", "main")
	write(t, dir, "search.txt", "search core\n")
	commit(t, dir, 5, "Search: core")
	write(t, dir, "ranking.txt", "ranking\n")
	commit(t, dir, 6, "Search: ranking")
	mustGit(t, dir, 7, "checkout", "-q", "main")
	mustGit(t, dir, 7, "merge", "-q", "--squash", "feature/search")
	commit(t, dir, 7, "Add search (#42)")

	// fix/typo: pushed, upstream configured, then deleted on the remote.
	mustGit(t, dir, 8, "checkout", "-q", "-b", "fix/typo", "main")
	write(t, dir, "app.txt", "v1 fixed typo\n")
	commit(t, dir, 8, "Fix typo")
	mustGit(t, dir, 8, "push", "-q", "origin", "fix/typo")
	mustGit(t, dir, 8, "branch", "-q", "--set-upstream-to=origin/fix/typo", "fix/typo")
	mustGit(t, dir, 8, "push", "-q", "origin", "--delete", "fix/typo")
	mustGit(t, dir, 8, "fetch", "-q", "--prune", "origin")

	// spike/old: unmerged work, pinned to 2020 → always stale.
	mustGit(t, dir, 9, "checkout", "-q", "-b", "spike/old", "main")
	write(t, dir, "spike.txt", "half an idea\n")
	commit(t, dir, 9, "Spike: half an idea")

	// wip: unmerged work; tests that need it active pass --stale-days 0.
	mustGit(t, dir, 10, "checkout", "-q", "-b", "wip", "main")
	write(t, dir, "wip.txt", "ongoing\n")
	commit(t, dir, 10, "WIP: ongoing")

	mustGit(t, dir, 11, "checkout", "-q", "main")
	return dir
}

// run invokes the CLI in-process.
func run(args ...string) (code int, stdout, stderr string) {
	var out, errBuf bytes.Buffer
	code = Run(args, &out, &errBuf)
	return code, out.String(), errBuf.String()
}

func TestVersionAndHelp(t *testing.T) {
	for _, argv := range [][]string{{"version"}, {"--version"}} {
		code, out, _ := run(argv...)
		if code != ExitOK || !strings.Contains(out, "leafrake 0.1.0") {
			t.Fatalf("%v → code=%d out=%q", argv, code, out)
		}
	}
	code, out, _ := run("help")
	if code != ExitOK || !strings.Contains(out, "Usage:") || !strings.Contains(out, "Exit codes") {
		t.Fatalf("help output wrong: code=%d\n%s", code, out)
	}
}

func TestScanClassifiesEveryCategory(t *testing.T) {
	dir := messyRepo(t)
	code, out, errOut := run("scan", dir)
	if code != ExitOK {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}
	for _, want := range []string{
		"MERGED         feature/login",
		"SQUASH-MERGED  feature/search",
		`matches squash commit`, `"Add search (#42)"`,
		"GONE           fix/typo",
		"upstream origin/fix/typo was deleted on the remote",
		"STALE          spike/old",
		"STALE          wip", // pinned 2020 dates: wip is stale by default too
		"PROTECTED      main",
		"deletable now: 2 (merged + squash-merged)",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("scan missing %q:\n%s", want, out)
		}
	}
}

func TestScanStaleDaysZeroMakesFreshWorkActive(t *testing.T) {
	dir := messyRepo(t)
	code, out, _ := run("scan", "--stale-days", "0", dir)
	if code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(out, "ACTIVE         spike/old") ||
		!strings.Contains(out, "ACTIVE         wip") {
		t.Fatalf("with --stale-days 0 unmerged work must be active:\n%s", out)
	}
}

func TestScanJSONShape(t *testing.T) {
	dir := messyRepo(t)
	code, out, errOut := run("scan", "--format", "json", dir)
	if code != ExitOK {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}
	var env struct {
		Tool    string `json:"tool"`
		Base    struct{ Name, Source string }
		Summary struct {
			Total        int `json:"total"`
			DeletableNow int `json:"deletable_now"`
		} `json:"summary"`
		Branches []struct {
			Name        string                    `json:"name"`
			Category    string                    `json:"category"`
			SquashMatch *struct{ Subject string } `json:"squash_match"`
			Evidence    []string                  `json:"evidence"`
		} `json:"branches"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if env.Tool != "leafrake" || env.Base.Name != "main" {
		t.Fatalf("envelope: %+v", env)
	}
	if env.Summary.Total != 6 || env.Summary.DeletableNow != 2 {
		t.Fatalf("summary: %+v", env.Summary)
	}
	byName := map[string]string{}
	for _, b := range env.Branches {
		byName[b.Name] = b.Category
		if b.Name == "feature/search" {
			if b.SquashMatch == nil || b.SquashMatch.Subject != "Add search (#42)" {
				t.Fatalf("squash proof: %+v", b)
			}
		}
		if len(b.Evidence) == 0 {
			t.Fatalf("branch %s has no evidence", b.Name)
		}
	}
	want := map[string]string{
		"feature/login": "merged", "feature/search": "squash-merged",
		"fix/typo": "gone", "spike/old": "stale", "main": "protected",
	}
	for name, cat := range want {
		if byName[name] != cat {
			t.Fatalf("%s: want %s, got %s", name, cat, byName[name])
		}
	}
}

func TestScanCheckExitsOneWhenDeletable(t *testing.T) {
	dir := messyRepo(t)
	if code, _, _ := run("scan", "--check", dir); code != ExitDead {
		t.Fatalf("--check with deletable branches: want exit 1, got %d", code)
	}
	// After protecting the deletable ones, --check passes.
	code, _, _ := run("scan", "--check", "--protect", "feature/*", dir)
	if code != ExitOK {
		t.Fatalf("--check with everything protected: want 0, got %d", code)
	}
}

func TestScanProtectGlob(t *testing.T) {
	dir := messyRepo(t)
	code, out, _ := run("scan", "--protect", "feature/*", dir)
	if code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(out, "PROTECTED      feature/login") ||
		!strings.Contains(out, `matches --protect "feature/*"`) {
		t.Fatalf("protect glob not applied:\n%s", out)
	}
}

func TestScanBareDefaultsToScan(t *testing.T) {
	dir := messyRepo(t)
	code, out, _ := run(dir) // bare path, no subcommand
	if code != ExitOK || !strings.Contains(out, "leafrake scan —") {
		t.Fatalf("bare path should scan: code=%d\n%s", code, out)
	}
}

func TestScanExplicitBaseFlag(t *testing.T) {
	dir := messyRepo(t)
	code, out, _ := run("scan", "--base", "main", dir)
	if code != ExitOK || !strings.Contains(out, "(base: main, from --base flag)") {
		t.Fatalf("--base not honored: code=%d\n%s", code, out)
	}
}

func TestCleanDryRunDeletesNothing(t *testing.T) {
	dir := messyRepo(t)
	code, out, _ := run("clean", dir)
	if code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(out, "dry run") ||
		!strings.Contains(out, "would delete  feature/login") ||
		!strings.Contains(out, "would delete  feature/search") {
		t.Fatalf("dry-run plan wrong:\n%s", out)
	}
	branches := mustGit(t, dir, 12, "branch", "--list")
	for _, name := range []string{"feature/login", "feature/search", "fix/typo"} {
		if !strings.Contains(branches, name) {
			t.Fatalf("dry run deleted %s! branches:\n%s", name, branches)
		}
	}
}

func TestCleanYesDeletesOnlyProvenBranches(t *testing.T) {
	dir := messyRepo(t)
	code, out, errOut := run("clean", "--yes", dir)
	if code != ExitOK {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}
	for _, want := range []string{
		"deleted       feature/login", "deleted       feature/search",
		"restore: git branch feature/login", "2 deleted, 0 failed",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q:\n%s", want, out)
		}
	}
	branches := mustGit(t, dir, 12, "branch", "--list")
	if strings.Contains(branches, "feature/") {
		t.Fatalf("proven branches still present:\n%s", branches)
	}
	for _, keep := range []string{"fix/typo", "spike/old", "wip", "main"} {
		if !strings.Contains(branches, keep) {
			t.Fatalf("%s should survive a default clean:\n%s", keep, branches)
		}
	}
}

func TestCleanRestoreHintActuallyRestores(t *testing.T) {
	// The printed restore command must genuinely resurrect the branch.
	dir := messyRepo(t)
	tip := mustGit(t, dir, 12, "rev-parse", "feature/search")
	if code, _, _ := run("clean", "--yes", dir); code != ExitOK {
		t.Fatal("clean failed")
	}
	mustGit(t, dir, 12, "branch", "feature/search", tip)
	if got := mustGit(t, dir, 12, "rev-parse", "feature/search"); got != tip {
		t.Fatalf("restored tip: want %s, got %s", tip, got)
	}
}

func TestCleanSelectGoneOptIn(t *testing.T) {
	dir := messyRepo(t)
	code, out, _ := run("clean", "--select", "gone", "--yes", dir)
	if code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(out, "deleted       fix/typo") {
		t.Fatalf("gone opt-in should delete fix/typo:\n%s", out)
	}
	branches := mustGit(t, dir, 12, "branch", "--list")
	if !strings.Contains(branches, "feature/login") {
		t.Fatalf("--select gone must not touch merged branches:\n%s", branches)
	}
}

func TestCleanSelectRejectsUnknownCategory(t *testing.T) {
	dir := messyRepo(t)
	code, _, errOut := run("clean", "--select", "merged,bogus", dir)
	if code != ExitUsage || !strings.Contains(errOut, `invalid --select "bogus"`) {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
}

func TestCleanJSONDryRun(t *testing.T) {
	dir := messyRepo(t)
	code, out, _ := run("clean", "--format", "json", dir)
	if code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	var env struct {
		DryRun    bool     `json:"dry_run"`
		Selection []string `json:"selection"`
		Branches  []struct {
			Name    string `json:"name"`
			Deleted bool   `json:"deleted"`
		} `json:"branches"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if !env.DryRun || len(env.Branches) != 2 || env.Branches[0].Deleted {
		t.Fatalf("clean JSON: %+v", env)
	}
}

func TestExplainSquashMergedBranch(t *testing.T) {
	dir := messyRepo(t)
	code, out, errOut := run("explain", "feature/search", dir)
	if code != ExitOK {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}
	for _, want := range []string{
		"leafrake explain — feature/search",
		"verdict: SQUASH-MERGED (deletable)",
		"squash patch-id: ",
		`"Add search (#42)"`,
		"delete with: leafrake clean --yes",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q:\n%s", want, out)
		}
	}
}

func TestExplainUnknownBranch(t *testing.T) {
	dir := messyRepo(t)
	code, _, errOut := run("explain", "no-such-branch", dir)
	if code != ExitRuntime || !strings.Contains(errOut, `no local branch named "no-such-branch"`) {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
}

func TestUsageErrors(t *testing.T) {
	dir := messyRepo(t)
	cases := [][]string{
		{"scan", "--format", "yaml", dir},
		{"clean", "--format", "yaml", dir},
		{"scan", dir, "extra-arg"},
		{"explain"},
		{"--bogus-flag"},
	}
	for _, argv := range cases {
		if code, _, _ := run(argv...); code != ExitUsage {
			t.Fatalf("%v: want exit %d, got %d", argv, ExitUsage, code)
		}
	}
}

func TestRuntimeErrorOutsideRepo(t *testing.T) {
	dir := t.TempDir() // not a git repository
	code, _, errOut := run("scan", dir)
	if code != ExitRuntime || !strings.Contains(errOut, "not a git repository") {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
	// A path that does not exist (often a typo'd subcommand routed through
	// the bare-path convenience) must be named directly, not via a git
	// chdir error.
	missing := filepath.Join(dir, "no-such-dir")
	code, _, errOut = run(missing)
	if code != ExitRuntime || !strings.Contains(errOut, fmt.Sprintf("path %q does not exist", missing)) {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
}

func TestWorktreeBranchSurvivesClean(t *testing.T) {
	dir := messyRepo(t)
	wt := filepath.Join(dir, "..", "wt-login")
	mustGit(t, dir, 12, "worktree", "add", "-q", wt, "feature/login")
	code, out, _ := run("clean", "--yes", dir)
	if code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	if strings.Contains(out, "deleted       feature/login") {
		t.Fatalf("worktree branch must never be deleted:\n%s", out)
	}
	branches := mustGit(t, dir, 12, "branch", "--list")
	if !strings.Contains(branches, "feature/login") {
		t.Fatal("feature/login should survive (checked out in a worktree)")
	}
}

func TestSquashWindowBoundsTheProof(t *testing.T) {
	// Land two more commits on main after the squash, so a 2-commit window
	// no longer reaches back to "Add search (#42)". Without proof the
	// branch must downgrade to stale — and must NOT be deletable.
	dir := messyRepo(t)
	write(t, dir, "later1.txt", "later 1\n")
	commit(t, dir, 12, "Later work 1")
	write(t, dir, "later2.txt", "later 2\n")
	commit(t, dir, 13, "Later work 2")

	code, out, _ := run("scan", dir) // default window still proves it
	if code != ExitOK || !strings.Contains(out, "SQUASH-MERGED  feature/search") {
		t.Fatalf("default window should prove the squash:\n%s", out)
	}
	code, out, _ = run("scan", "--squash-window", "2", dir)
	if code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	if strings.Contains(out, "SQUASH-MERGED") {
		t.Fatalf("window 2 must not reach the squash commit:\n%s", out)
	}
	if !strings.Contains(out, "STALE          feature/search") {
		t.Fatalf("unproven branch should fall through to stale:\n%s", out)
	}
	if !strings.Contains(out, "deletable now: 1 (merged + squash-merged)") {
		t.Fatalf("only feature/login should stay deletable:\n%s", out)
	}
}

func TestDetectsBaseFromOriginHead(t *testing.T) {
	dir := messyRepo(t)
	// Rename the local conventional names away and set origin/HEAD instead.
	mustGit(t, dir, 12, "push", "-q", "origin", "main")
	mustGit(t, dir, 12, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	mustGit(t, dir, 12, "branch", "-m", "main", "mainline")
	// origin/HEAD names "main", which no longer exists locally → the remote
	// ref itself becomes the base.
	code, out, _ := run("scan", "--stale-days", "0", dir)
	if code != ExitOK {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	if !strings.Contains(out, "(base: origin/main, from origin/HEAD)") {
		t.Fatalf("origin/HEAD base detection failed:\n%s", out)
	}
	if !strings.Contains(out, "MERGED         feature/login") {
		t.Fatalf("comparisons against a remote base must still work:\n%s", out)
	}
}
