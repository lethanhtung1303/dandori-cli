# 2026-04-25 — Hackday Prep (5 phases) + Snapshot Pre-existence Fix

## Summary

Shipped all 5 phases of `plans/260421-2031-dandori-cli-hackday-prep/` via TDD.
Discovered + fixed a regression of the Phase-01 tokens=0 bug that surfaced
only when running Claude in a brand-new sandbox directory.

## Phases delivered

| # | Phase | Outcome |
|---|---|---|
| 01 | Tailer timing fix | Phase-A ticker + Phase-B post-exit drain (10s timeout, 750ms idle grace) |
| 02 | Demo seed + DB reset | `dandori demo --reset --seed --use --restore`, idempotent via seedTag |
| 03 | `analytics all` | 4-block snapshot: cost / leaderboard / quality / alerts |
| 04 | Engineer/Department group-by | `analytics cost --by engineer\|department`, mix leaderboard with `(human)` rows |
| 05 | E2E rehearsal | `scripts/hackday-rehearsal.sh dry\|live`, demo script, sandbox folder |

All TDD — RED test first, then GREEN. Full `go test ./...` green.

## The regression — tokens=0 on real Claude in sandbox

### Symptom

First live rehearsal completed end-to-end (Claude wrote `main.go` in
`demo-workspace/260425-0917-live/`, Jira sync OK), but the wrapper logged:

```
Tokens: 0 in / 0 out
Cost: $0.0000
```

This is the exact symptom Phase 01 was supposed to prevent.

### Root cause

`SnapshotSessionDir(cwd)` called `getClaudeProjectDir(cwd)` which checked
`os.Stat(~/.claude/projects/<encoded-cwd>)` and returned `""` if missing.

In a fresh sandbox subdir, that directory does not exist yet — Claude
creates it lazily on first run. Sequence:

1. Wrapper `cd`s into `demo-workspace/260425-0924-live/`
2. Wrapper calls `SnapshotSessionDir(cwd)` → claudeDir not yet created → `Dir = ""`
3. Wrapper spawns `claude -p ...` → Claude creates `~/.claude/projects/-Users-...-260425-0924-live/` and writes JSONL there
4. Tailer enters `TailSessionLog` → snapshot.Dir is `""` → bails immediately
5. Result: tokens=0

The earlier 2026-04-19 symlink fix only addressed `/tmp → /private/tmp`.
This one is the chicken-and-egg problem: snapshot runs before Claude has
ever created the project dir.

### Fix

Split the path computation from the existence check
(`internal/wrapper/snapshot.go`):

```go
// expectedClaudeProjectDir computes the path without requiring it to exist.
// SnapshotSessionDir uses this so the tailer can poll for the dir/file.
func expectedClaudeProjectDir(cwd string) string { ... }

// getClaudeProjectDir keeps the existence-checked behaviour for callers
// that need it.
func getClaudeProjectDir(cwd string) string { ... }
```

`SnapshotSessionDir` now sets `Dir` even when the directory is missing.
`os.ReadDir` returns an error in that case → empty Files map (correct,
nothing to compare against). The tailer's `GetSessionLogPath` calls
`DetectSessionID` which `os.ReadDir`s lazily — succeeds once Claude
creates the dir.

### Verification — 3× back-to-back live runs

| Run | Stage 4 (Claude) | Total | Tokens out | Cost |
|---|---|---|---|---|
| 1 | 30.0s | 34.9s | 2291 | $1.47 |
| 2 | 30.2s | 33.8s | 3006 | $1.65 |
| 3 | 14.8s | 18.3s | 621 | $1.14 |

3/3 capture tokens > 0. p95 Stage 4 ≈ 30s, comfortably under the 150s
demo budget.

## Files changed

- `internal/wrapper/snapshot.go` — split `getClaudeProjectDir` /
  `expectedClaudeProjectDir`; `SnapshotSessionDir` no longer drops Dir on
  missing directory.
- `internal/wrapper/snapshot_test.go` — regression test
  `TestSnapshotSessionDir_CreatesDirPathEvenIfMissing`.
- `cmd/demo.go` — DANDORI_DB env override respected.
- `internal/db/quality.go` — `COALESCE(agent_name, '(human)')` for
  human-only rows in quality stats.
- `internal/analytics/all.go`, `alerts.go` — Phase-03 unified snapshot
  + threshold alerts.
- `cmd/analytics_all.go`, `cmd/analytics_cost.go` — Phase-03/04 commands.
- `scripts/hackday-rehearsal.sh` — sandbox per-session dir, zsh
  `EPOCHREALTIME` via `zmodload zsh/datetime`.
- `demo-workspace/{.gitignore,README.md}` — sandbox structure.
- `docs/hackday-demo-script.md` — stage-by-stage narration.
- `Makefile` — `rehearsal`, `rehearsal-live`, `rehearsal-e2e` targets.

## Lessons

1. Existence checks at the wrong layer hide otherwise-correct logic.
   The path computation is pure; existence is the tailer's concern (it
   already polls). Coupling them broke the fresh-sandbox case.
2. End-to-end with real Claude exposes the bugs `go test` cannot —
   especially around process lifecycle and filesystem race conditions.
3. The Phase-01 fix was correct but assumed the project dir already
   existed. A "Phase-01 truly complete" test would have spawned Claude
   in a never-used cwd.
