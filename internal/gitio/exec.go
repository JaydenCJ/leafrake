// Package gitio talks to the local git binary and parses its plumbing
// output. All parsers are pure functions over bytes so they can be tested
// against captured fixtures; only the thin Git runner shells out.
package gitio

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// refFormat renders one line per local branch: seven fields separated by
// the ASCII unit separator (0x1f). %(subject) is the first message line,
// which cannot contain a newline, so newline is a safe record separator.
const refFormat = "%(HEAD)%1f%(refname:short)%1f%(objectname)%1f" +
	"%(upstream:short)%1f%(upstream:track)%1f%(committerdate:iso-strict)%1f%(subject)"

// baseLogFormat renders one line per base commit: hash, subject, and
// committer date, again unit-separator delimited.
const baseLogFormat = "%H%x1f%s%x1f%cI"

// Git runs git commands inside Dir. The zero value runs in the current
// working directory.
type Git struct {
	Dir string
}

// run executes git with hardened flags so user configuration (pagers,
// signature display) cannot change the output shape, and fails on any
// non-zero exit.
func (g Git) run(stdin []byte, args ...string) ([]byte, error) {
	out, code, err := g.runExit(stdin, args...)
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, fmt.Errorf("git %s: exit status %d", args[0], code)
	}
	return out, nil
}

// runExit is like run but surfaces the exit code, for plumbing commands
// such as `merge-base --is-ancestor` that answer questions via exit status.
// err is non-nil only when git produced a real error message or could not
// be started at all.
func (g Git) runExit(stdin []byte, args ...string) ([]byte, int, error) {
	full := append([]string{
		"-c", "log.showSignature=false",
		"-c", "core.pager=cat",
		"-c", "color.ui=false",
	}, args...)
	cmd := exec.Command("git", full...)
	cmd.Dir = g.Dir
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	err := cmd.Run()
	if err == nil {
		return out.Bytes(), 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if msg := strings.TrimSpace(errBuf.String()); msg != "" {
			return nil, exitErr.ExitCode(), fmt.Errorf("git %s: %s", args[0], firstLine(msg))
		}
		return out.Bytes(), exitErr.ExitCode(), nil
	}
	return nil, -1, fmt.Errorf("git %s: %v", args[0], err)
}

// RepoTop returns the absolute path of the repository working-tree root.
func (g Git) RepoTop() (string, error) {
	out, err := g.run(nil, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// RefsRaw returns one refFormat line per local branch, ready for ParseRefs.
func (g Git) RefsRaw() ([]byte, error) {
	return g.run(nil, "for-each-ref", "refs/heads", "--format="+refFormat)
}

// WorktreesRaw returns `git worktree list --porcelain` output, ready for
// ParseWorktrees.
func (g Git) WorktreesRaw() ([]byte, error) {
	return g.run(nil, "worktree", "list", "--porcelain")
}

// OriginHead returns the branch origin/HEAD points at (e.g. "origin/main"),
// or "" when the symbolic ref is not set. Never touches the network.
func (g Git) OriginHead() (string, error) {
	out, code, err := g.runExit(nil, "symbolic-ref", "-q", "refs/remotes/origin/HEAD")
	if code != 0 {
		return "", nil // unset is normal for repos that never cloned
	}
	if err != nil {
		return "", err
	}
	return strings.TrimPrefix(strings.TrimSpace(string(out)), "refs/remotes/"), nil
}

// ResolveRef returns the commit hash a ref points at, or ok=false when the
// ref does not exist.
func (g Git) ResolveRef(ref string) (hash string, ok bool, err error) {
	out, code, err := g.runExit(nil, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	if code != 0 {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return strings.TrimSpace(string(out)), true, nil
}

// MergeBase returns the best common ancestor of a and b, or ok=false when
// the histories are unrelated.
func (g Git) MergeBase(a, b string) (hash string, ok bool, err error) {
	out, code, err := g.runExit(nil, "merge-base", a, b)
	if code == 1 {
		return "", false, nil // no common ancestor
	}
	if err != nil {
		return "", false, err
	}
	if code != 0 {
		return "", false, fmt.Errorf("git merge-base: exit status %d", code)
	}
	return strings.TrimSpace(string(out)), true, nil
}

// IsAncestor reports whether commit a is an ancestor of commit b.
func (g Git) IsAncestor(a, b string) (bool, error) {
	_, code, err := g.runExit(nil, "merge-base", "--is-ancestor", a, b)
	switch code {
	case 0:
		return true, nil
	case 1:
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return false, fmt.Errorf("git merge-base --is-ancestor: exit status %d", code)
}

// DiffPatchID computes the patch-id of `git diff a b` — the whole branch
// squashed into a single hypothetical commit. Returns "" when the diff is
// empty (the branch changes nothing relative to a).
func (g Git) DiffPatchID(a, b string) (string, error) {
	diff, err := g.run(nil, "diff", "--no-ext-diff", "--no-textconv", a, b)
	if err != nil {
		return "", err
	}
	if len(bytes.TrimSpace(diff)) == 0 {
		return "", nil
	}
	out, err := g.run(diff, "patch-id", "--stable")
	if err != nil {
		return "", err
	}
	return FirstPatchID(out), nil
}

// BasePatchIDsRaw streams the last `window` first-parent commits of base
// through `git patch-id --stable`, yielding one "patch-id commit-hash" line
// per non-empty, non-merge commit. Ready for ParsePatchIDs.
func (g Git) BasePatchIDsRaw(base string, window int) ([]byte, error) {
	log, err := g.run(nil, "log", "--first-parent", "-p", "--no-ext-diff",
		"--no-textconv", "--format=%H", "-n", strconv.Itoa(window), base, "--")
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(log)) == 0 {
		return nil, nil
	}
	return g.run(log, "patch-id", "--stable")
}

// BaseLogRaw returns hash/subject/date metadata for the last `window`
// first-parent commits of base, ready for ParseBaseLog.
func (g Git) BaseLogRaw(base string, window int) ([]byte, error) {
	return g.run(nil, "log", "--first-parent", "--format="+baseLogFormat,
		"-n", strconv.Itoa(window), base, "--")
}

// AheadBehind counts commits only on the branch (ahead) and only on base
// (behind) via `git rev-list --left-right --count base...tip`.
func (g Git) AheadBehind(base, tip string) (ahead, behind int, err error) {
	out, err := g.run(nil, "rev-list", "--left-right", "--count", base+"..."+tip)
	if err != nil {
		return 0, 0, err
	}
	fields := strings.Fields(string(out))
	if len(fields) != 2 {
		return 0, 0, fmt.Errorf("git rev-list --count: unexpected output %q", string(out))
	}
	behind, err = strconv.Atoi(fields[0])
	if err != nil {
		return 0, 0, err
	}
	ahead, err = strconv.Atoi(fields[1])
	if err != nil {
		return 0, 0, err
	}
	return ahead, behind, nil
}

// DeleteBranch deletes a local branch after re-verifying that its tip is
// still exactly the hash the scan recorded, so a branch that moved between
// scan and delete is never touched.
func (g Git) DeleteBranch(name, expectedTip string) error {
	hash, ok, err := g.ResolveRef("refs/heads/" + name)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("branch %q no longer exists", name)
	}
	if hash != expectedTip {
		return fmt.Errorf("branch %q moved (now %.7s, scanned %.7s); re-run scan",
			name, hash, expectedTip)
	}
	_, err = g.run(nil, "branch", "-D", "--", name)
	return err
}

func firstLine(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return s[:idx]
	}
	return s
}
