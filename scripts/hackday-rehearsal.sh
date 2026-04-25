#!/usr/bin/env zsh
# Hackday demo rehearsal — runs the 6-step flow against demo.db.
# Usage:
#   ./scripts/hackday-rehearsal.sh          # dry (no real Claude)
#   ./scripts/hackday-rehearsal.sh live     # real Claude on CLITEST-1
set -euo pipefail
zmodload zsh/datetime

REPO_ROOT="${REPO_ROOT:-$(pwd)}"
DANDORI="${DANDORI:-$REPO_ROOT/bin/dandori}"
MODE="${1:-dry}"

if [[ ! -x "$DANDORI" ]]; then
  echo "dandori binary not found at $DANDORI — run 'make build' first" >&2
  exit 1
fi

# Session sandbox — Claude writes files here, not into the source tree.
# One subdir per rehearsal; kept for post-mortem review.
SESSION_ID="$(date +%y%m%d-%H%M)-$MODE"
SESSION_DIR="$REPO_ROOT/demo-workspace/$SESSION_ID"
mkdir -p "$SESSION_DIR"
echo "Session workspace: $SESSION_DIR"

cleanup() {
  "$DANDORI" demo --restore >/dev/null 2>&1 || true
}
trap cleanup EXIT

started=$EPOCHREALTIME

echo "[1/6] Reset demo DB + seed blog scenario…"
"$DANDORI" demo --reset --seed --use

if [[ "$MODE" == "live" ]]; then
  echo "[2/6] Running REAL Claude on CLITEST-1 in $SESSION_DIR…"
  (cd "$SESSION_DIR" && "$DANDORI" task run CLITEST-1)
else
  echo "[2/6] Dry-run CLITEST-1 (skip Claude)…"
  "$DANDORI" task run CLITEST-1 --dry-run || echo "  (dry-run unsupported — continuing)"
fi

echo
echo "[3/6] Last run tokens…"
"$DANDORI" analytics runs --limit 1

echo
echo "[4/6] Analytics all (4-block snapshot)…"
"$DANDORI" analytics all --since 30

echo
echo "[5/6] Cost by engineer…"
"$DANDORI" analytics cost --by engineer

echo
echo "[6/6] Cost by department…"
"$DANDORI" analytics cost --by department

ended=$EPOCHREALTIME
elapsed=$(( ended - started ))
printf "\nRehearsal finished in %.1fs (mode=%s)\n" "$elapsed" "$MODE"
echo "Artifacts in: $SESSION_DIR"
