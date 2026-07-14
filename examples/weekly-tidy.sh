#!/usr/bin/env bash
# A conservative weekly tidy-up you can run from cron or by hand:
#
#   1. prune remote-tracking refs so [gone] state is current,
#   2. show the full classification with evidence,
#   3. delete only the *proven* categories (merged + squash-merged).
#
# Nothing gone or stale is ever touched — review those by hand with
# `leafrake scan` / `leafrake explain <branch>`.
#
# Usage: bash examples/weekly-tidy.sh [repo-path]
set -euo pipefail

REPO="${1:-.}"

# Refresh tracking state; skip silently when there is no remote.
if git -C "$REPO" remote get-url origin > /dev/null 2>&1; then
  git -C "$REPO" fetch --prune origin
fi

leafrake scan "$REPO"

# Dry run first, then delete. `clean` only ever selects merged and
# squash-merged unless you opt in to more via --select.
leafrake clean "$REPO"
leafrake clean --yes "$REPO"
