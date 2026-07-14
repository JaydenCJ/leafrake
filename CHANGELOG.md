# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] - 2026-07-12

### Added

- Branch classification into six categories — merged, squash-merged, gone,
  stale, active, protected — with per-branch evidence lines for every
  verdict.
- Squash-merge proof by patch-id: each branch's would-be squash commit
  (diff from merge-base to tip) is hashed with `git patch-id --stable` and
  matched against a bounded first-parent index of the base branch
  (`--squash-window`, default 1000); matches quote the landing commit's
  hash, subject, and date.
- Merged proof via `git merge-base --is-ancestor` plus a net-zero-diff rule
  for branches whose tree equals their merge-base tree.
- Zero-configuration base detection: `origin/HEAD` (as a local symbolic
  ref, no network), then `main`/`master`/`trunk`/`develop`, with `--base`
  override supporting remote refs.
- Automatic protection for the base branch, the checked-out branch, and
  branches checked out in linked worktrees, plus repeatable `--protect`
  globs (path.Match semantics).
- `scan` subcommand with text and stable JSON (`schema_version: 1`) output
  and a `--check` exit-code gate for hooks.
- `clean` subcommand: dry run by default, `--yes` to delete, category
  opt-in via `--select` (gone and stale are never deleted by default),
  tip re-verification before each deletion, and a printed
  `restore: git branch <name> <hash>` line per deleted branch.
- `explain` subcommand printing the full evidence dossier for one branch
  (upstream state, merge-base, ahead/behind, squash patch-id, verdict).
- Staleness rule with `--stale-days` (default 90, 0 disables).
- Runnable examples (`examples/make-messy-repo.sh`,
  `examples/weekly-tidy.sh`) and a classification-rules reference
  (`docs/classification.md`).
- 90 deterministic offline tests (unit + in-process CLI integration against
  fabricated git repositories with a local bare "remote") and
  `scripts/smoke.sh`.

[0.1.0]: https://github.com/JaydenCJ/leafrake/releases/tag/v0.1.0
