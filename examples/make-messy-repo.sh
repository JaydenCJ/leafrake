#!/usr/bin/env bash
# Fabricates a demo repository with one branch of every leafrake category:
#
#   main             base (holds the squash of feature/search)
#   feature/login    merged via a real merge commit
#   feature/search   squash-merged ("Add search (#42)") — invisible to --merged
#   fix/typo         upstream deleted on a local bare "remote" → gone
#   spike/old        unmerged, last commit years ago → stale
#   feature/wip      unmerged, committed today → active
#
# Usage: bash examples/make-messy-repo.sh /tmp/leafrake-demo
# Entirely offline: the "remote" is a bare repository next to the clone.
set -euo pipefail

TARGET="${1:?usage: make-messy-repo.sh <target-dir>}"
rm -rf "$TARGET"
mkdir -p "$TARGET"
# Resolve to an absolute path: the remote URL is stored in the clone's config
# and would otherwise resolve relative to the repo directory, not the caller.
TARGET="$(cd "$TARGET" && pwd)"
REPO="$TARGET/repo"
ORIGIN="$TARGET/origin.git"

export GIT_CONFIG_GLOBAL=/dev/null
export GIT_CONFIG_SYSTEM=/dev/null
export GIT_AUTHOR_NAME="Dev"
export GIT_AUTHOR_EMAIL="dev@example.test"
export GIT_COMMITTER_NAME="Dev"
export GIT_COMMITTER_EMAIL="dev@example.test"

commit_on() {
  # commit_on <date> <message>: stage everything, commit with a pinned date.
  local date="$1" message="$2"
  git -C "$REPO" add -A
  GIT_AUTHOR_DATE="$date" GIT_COMMITTER_DATE="$date" \
    git -C "$REPO" commit -q --no-gpg-sign --allow-empty -m "$message"
}

git init -q --bare "$ORIGIN"
git init -q "$REPO"
git -C "$REPO" checkout -q -b main
git -C "$REPO" remote add origin "$ORIGIN"

printf 'v1\n' > "$REPO/app.txt"
commit_on 2026-01-05T10:00:00+00:00 "Initial commit"

# feature/login — landed with a real merge commit.
git -C "$REPO" checkout -q -b feature/login
printf 'login v1\n' > "$REPO/login.txt"
commit_on 2026-01-06T10:00:00+00:00 "Login: skeleton"
printf 'login v2\n' > "$REPO/login.txt"
commit_on 2026-01-07T10:00:00+00:00 "Login: polish"
git -C "$REPO" checkout -q main
GIT_AUTHOR_DATE=2026-01-08T10:00:00+00:00 GIT_COMMITTER_DATE=2026-01-08T10:00:00+00:00 \
  git -C "$REPO" merge -q --no-ff --no-edit feature/login

# feature/search — squash-merged, the case `git branch --merged` misses.
git -C "$REPO" checkout -q -b feature/search main
printf 'search core\n' > "$REPO/search.txt"
commit_on 2026-02-02T10:00:00+00:00 "Search: core"
printf 'ranking\n' > "$REPO/ranking.txt"
commit_on 2026-02-03T10:00:00+00:00 "Search: ranking"
git -C "$REPO" checkout -q main
git -C "$REPO" merge -q --squash feature/search > /dev/null
commit_on 2026-02-04T10:00:00+00:00 "Add search (#42)"

# fix/typo — pushed, then its remote branch deleted (post-PR cleanup).
git -C "$REPO" checkout -q -b fix/typo main
printf 'v1 fixed typo\n' > "$REPO/app.txt"
commit_on 2026-03-01T10:00:00+00:00 "Fix typo"
git -C "$REPO" push -q origin fix/typo
git -C "$REPO" branch -q --set-upstream-to=origin/fix/typo fix/typo
git -C "$REPO" push -q origin --delete fix/typo
git -C "$REPO" fetch -q --prune origin

# spike/old — abandoned experiment.
git -C "$REPO" checkout -q -b spike/old main
printf 'half an idea\n' > "$REPO/spike.txt"
commit_on 2024-11-20T10:00:00+00:00 "Spike: half an idea"

# feature/wip — live work from today.
git -C "$REPO" checkout -q -b feature/wip main
printf 'ongoing\n' > "$REPO/wip.txt"
commit_on "$(date -u +%Y-%m-%dT10:00:00+00:00)" "WIP: streaming exports"

git -C "$REPO" checkout -q main
echo "demo repository ready: $REPO"
