# Classification rules

leafrake assigns every local branch exactly one category. Rules are checked
in a fixed precedence order; the first proof wins, and later signals (a gone
upstream on an already-proven branch) are still reported as extra evidence.

## Precedence

1. **protected** — the base branch, the branch checked out in the current
   worktree (HEAD), any branch checked out in another linked worktree, and
   anything matching a `--protect` pattern. Protection short-circuits every
   other rule: even a provably merged HEAD is never touched.
2. **merged** — proof, two forms:
   - the branch tip is an ancestor of the base
     (`git merge-base --is-ancestor`), i.e. a regular merge or fast-forward
     landed it; this is the case `git branch --merged` also sees;
   - the branch's tree is identical to its merge-base tree (net-zero diff):
     the branch changes nothing, so deleting it loses nothing.
3. **squash-merged** — proof by patch-id. See below.
4. **gone** — the branch has a configured upstream and
   `for-each-ref %(upstream:track)` reports `[gone]`: the remote branch was
   deleted (typically post-PR cleanup), but the local content could not be
   proven merged. Common causes: a rebase-merge, or a squash that landed
   with conflict resolutions. Deleting requires `--select gone`.
5. **stale** — no proof, upstream intact or unset, and the tip's committer
   date is at least `--stale-days` (default 90) days old. Deleting requires
   `--select stale`.
6. **active** — everything else. Never deletable.

Only **merged** and **squash-merged** are deleted by a default `leafrake
clean`; they are the two categories where the content provably exists on the
base branch. **gone** and **stale** are heuristics, so they are opt-in.

## The squash-merge proof

`git merge --squash` (and the squash button on every major forge) lands a
branch as one new commit with no merge parent, so the branch tip is *not* an
ancestor of the base and `git branch --merged` never lists it. leafrake
proves the merge a different way:

1. Compute the branch's **would-be squash commit**: the single diff from
   `merge-base(base, branch)` to the branch tip, hashed with
   `git patch-id --stable`. The patch-id ignores hunk offsets and context
   shifts, so unrelated later commits on base do not break it.
2. Stream the last `--squash-window` (default 1000) first-parent commits of
   the base branch through one `git log -p | git patch-id --stable` pass,
   building a patch-id → commit index. Merge commits and empty commits
   produce no patch and drop out naturally.
3. If the branch's patch-id is in the index, the branch is squash-merged —
   and the matching commit's hash, subject, and date are quoted as the
   evidence you see in `scan` and `explain`.

### Honest limits

- A squash that required **conflict resolution** (or was edited before
  landing) produces a different diff, hence a different patch-id: no match.
  The branch falls through to gone/stale instead of being wrongly deleted —
  leafrake fails safe, never loose.
- **Rebase-merges** land each commit separately; the single-diff proof does
  not apply. Per-commit matching is on the roadmap.
- The index is bounded by `--squash-window`; a squash older than the window
  is not found. Raise the window for very old branches.

## Base branch detection

Zero configuration, first hit wins:

1. `--base <branch>` if given (may be a remote ref like `origin/main`).
2. What `origin/HEAD` points at — preferring the local branch of the same
   name when it exists, otherwise the remote ref itself. Never touches the
   network; `origin/HEAD` is a local symbolic ref.
3. The first existing local branch among `main`, `master`, `trunk`,
   `develop`.

If none of those apply, leafrake refuses to guess and asks for `--base`.

## Deletion safety

- `clean` without `--yes` is always a dry run.
- Every deletion re-resolves the branch tip immediately before running
  `git branch -D`; if the branch moved since the scan, it is skipped with an
  error instead of deleted.
- Every deletion prints `restore: git branch <name> <hash>`. The commits
  stay in the object database until git garbage-collects unreachable
  objects, so the restore line genuinely works.
