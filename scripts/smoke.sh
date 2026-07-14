#!/usr/bin/env bash
# End-to-end smoke test for leafrake: builds the binary, fabricates a
# deterministic repository with one branch of every category (including a
# local bare "remote" for the gone case), and asserts on real CLI output.
# No network, idempotent, finishes in seconds.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORKDIR="$(mktemp -d)"
trap 'rm -rf "$WORKDIR"' EXIT

fail() {
  echo "SMOKE FAIL: $*" >&2
  exit 1
}

BIN="$WORKDIR/leafrake"
REPO="$WORKDIR/demo/repo"

echo "1. build"
(cd "$ROOT" && go build -o "$BIN" ./cmd/leafrake) || fail "go build failed"

echo "2. version matches manifest"
"$BIN" --version | grep -qx "leafrake 0.1.0" || fail "--version mismatch"

echo "3. fabricate a messy repository (examples/make-messy-repo.sh)"
bash "$ROOT/examples/make-messy-repo.sh" "$WORKDIR/demo" > /dev/null

echo "4. scan classifies every category with evidence"
OUT="$("$BIN" scan "$REPO")"
echo "$OUT" | grep -q "MERGED         feature/login" || fail "merged branch missing"
echo "$OUT" | grep -q "SQUASH-MERGED  feature/search" || fail "squash-merged branch missing"
echo "$OUT" | grep -q 'matches squash commit .* "Add search (#42)"' || fail "squash proof missing"
echo "$OUT" | grep -q "GONE           fix/typo" || fail "gone branch missing"
echo "$OUT" | grep -q "upstream origin/fix/typo was deleted on the remote" || fail "gone evidence missing"
echo "$OUT" | grep -q "STALE          spike/old" || fail "stale branch missing"
echo "$OUT" | grep -q "ACTIVE         feature/wip" || fail "active branch missing"
echo "$OUT" | grep -q "deletable now: 2" || fail "deletable count wrong"

echo "5. git's own --merged misses the squash (the reason leafrake exists)"
git -C "$REPO" branch --merged main | grep -q "feature/search" \
  && fail "fixture broken: git --merged should NOT list the squash branch"

echo "6. JSON output is machine-readable and correct"
JSON="$("$BIN" scan --format json "$REPO")"
echo "$JSON" | grep -q '"tool": "leafrake"' || fail "json envelope missing"
echo "$JSON" | grep -q '"deletable_now": 2' || fail "json deletable count wrong"
echo "$JSON" | grep -q '"category": "squash-merged"' || fail "json squash category missing"
echo "$JSON" | grep -q '"subject": "Add search (#42)"' || fail "json squash proof missing"

echo "7. explain prints the full dossier"
"$BIN" explain feature/search "$REPO" | grep -q "verdict: SQUASH-MERGED (deletable)" \
  || fail "explain verdict missing"

echo "8. scan --check gates with exit 1"
set +e
"$BIN" scan --check "$REPO" > /dev/null
[ $? -eq 1 ] || fail "--check should exit 1 while deletable branches exist"
set -e

echo "9. clean is a dry run by default"
"$BIN" clean "$REPO" | grep -q "would delete  feature/login" || fail "dry-run plan missing"
git -C "$REPO" rev-parse --verify -q refs/heads/feature/search > /dev/null \
  || fail "dry run deleted a branch"

echo "10. clean --yes deletes exactly the proven branches"
TIP="$(git -C "$REPO" rev-parse feature/search)"
OUT="$("$BIN" clean --yes "$REPO")"
echo "$OUT" | grep -q "2 deleted, 0 failed" || fail "clean summary wrong"
git -C "$REPO" rev-parse --verify -q refs/heads/feature/login > /dev/null \
  && fail "merged branch should be gone"
git -C "$REPO" rev-parse --verify -q refs/heads/fix/typo > /dev/null \
  || fail "gone branch must survive a default clean"
git -C "$REPO" rev-parse --verify -q refs/heads/spike/old > /dev/null \
  || fail "stale branch must survive a default clean"

echo "11. the printed restore command genuinely resurrects a branch"
git -C "$REPO" branch feature/search "$TIP"
[ "$(git -C "$REPO" rev-parse feature/search)" = "$TIP" ] || fail "restore hint broken"

echo "12. usage errors exit 2"
set +e
"$BIN" scan --format yaml "$REPO" > /dev/null 2>&1
[ $? -eq 2 ] || fail "bad --format should exit 2"
set -e

echo "SMOKE OK"
