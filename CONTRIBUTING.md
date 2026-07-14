# Contributing to leafrake

Issues, discussions and pull requests are all welcome.

## Getting started

You need Go ≥1.22 and git ≥2.31 (for stable `%(upstream:track)` output);
nothing else.

```bash
git clone https://github.com/JaydenCJ/leafrake && cd leafrake
go build ./...
go test ./...
bash scripts/smoke.sh
```

`scripts/smoke.sh` builds the binary, fabricates a deterministic repository
with one branch of every category (using a local bare repo as the
"remote"), and asserts on real CLI output across every subcommand; it must
finish by printing `SMOKE OK`.

## Before you open a pull request

1. `gofmt -l .` reports nothing (formatting is enforced).
2. `go vet ./...` passes with no findings.
3. `go test ./...` passes (90 deterministic tests, no network).
4. `bash scripts/smoke.sh` prints `SMOKE OK`.
5. Add tests for behavior changes; keep logic in pure, unit-testable
   modules (classification and parsing never shell out — only `gitio.Git`
   does).

## Ground rules

- Keep dependencies at zero; adding one needs strong justification in the
  PR.
- No network calls, ever — leafrake's only external interface is the local
  `git` binary. No telemetry. Base detection reads `origin/HEAD` as a local
  symbolic ref and must stay offline.
- Deletion safety is the product: anything that widens what gets deleted
  needs a test proving the protected cases (HEAD, worktrees, base,
  `--protect`) still survive, and evidence lines explaining the new rule.
- Code comments and doc comments are written in English.
- Determinism first: identical repository state must produce byte-identical
  reports, including all orderings.

## Reporting bugs

Include the output of `leafrake version`, the full command you ran, the
scan output, and — for misclassifications — `leafrake explain <branch>`
plus `git log --oneline -3 <branch>` and `git merge-base <base> <branch>`,
since that is exactly what the classifier sees. Never include repository
content you cannot share; the evidence lines are usually enough.

## Security

Please do not open public issues for security problems; use GitHub's
private vulnerability reporting on this repository instead.
