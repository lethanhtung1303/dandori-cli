# 2026-04-26 — Tracking Layer-3 Completion (4 phases live)

## Summary

Closed 4 of 5 originally identified Layer-3 tracking gaps from `plans/260425-2100-tracking-layer-3-completion/`.
All TDD (RED → GREEN). Full `go test ./...` green. Live verified against
fooknt.atlassian.net (CLITEST project / space).

## Phases delivered

| # | Phase | Outcome | Live? |
|---|---|---|---|
| 01 | Tool/skill events from JSONL | Tailer parses `tool.use` / `tool.result` / `skill.list` / `skill.invoke` from session log; payload keeps keys + sizes only (no values) | ✅ tool.use + tool.result observed in events table |
| 02 | Confluence read events | `taskcontext.Build` records `confluence.read{page_id, title, char_count, version}` per page fetched | ✅ 1 event for CLITEST-1 with linked test page |
| 03 | Task iteration tracking | `DetectIteration` (workflow-agnostic via `statusCategory.key`) + poller integration + wrapper emits `task.iteration.end` | ⚠️ via integration test (httptest); CLI daemon command not yet wired |
| 05 | Analytics queries | `dandori analytics tools \| context \| iterations` with `--since / --top / --by / --format` flags | ✅ all three return real data, JSON format valid |
| 04 core | Bug-link Jira | `ParseDescriptionTags`, `ParseLinkCandidates`, `DetectBugLinks` + `BugLinkResolver` interface + DB resolvers (`FindRunByPrefix`, `BugEventExists`) | core only — auto-poller wiring deferred |

## Architecture decisions

- **`statusCategory.key`** instead of status name → workflow-agnostic detection (works on any Jira board regardless of custom workflow)
- **`BugLinkResolver` interface** in `internal/jira/` so the package can consume DB without an import cycle
- **Pure detection functions** + test-double resolvers → table-driven unit tests, no DB dependency
- **Analytics SQL on SQLite stays in `internal/db/`** (kept `internal/analytics/` for the Postgres pgx pool used by the server)
- **Privacy-first event payloads:** Confluence stores `char_count` not body; tool.use stores `input_keys` not values; run-prefix matching requires ≥12 hex to avoid collision
- **Tracking-must-not-break:** every recorder failure is `slog.Warn` + continue — never aborts the parent flow

## Live verify trace

```
$ dandori run --task CLITEST-1 -- claude --print "List files then read README"
$ sqlite3 ~/.dandori/local.db "SELECT event_type, COUNT(*) FROM events WHERE run_id='7a66ec59...' GROUP BY event_type"
tool.use|2
tool.result|2

$ # created Confluence page + remote-linked to CLITEST-1
$ dandori task run CLITEST-1 -- claude --print "Acknowledge"
$ sqlite3 ~/.dandori/local.db "SELECT event_type, data FROM events WHERE run_id='5be1c143...'"
confluence.read|{"char_count":74,"page_id":"2523154","title":"Live Verify Phase 02 - 1777165122","version":1}

$ dandori analytics tools
TOOL  USES  SUCCESS%  LAST USED
Read  1     100.0%    2026-04-26 07:47
Bash  1     100.0%    2026-04-26 07:47

$ dandori analytics context
PAGE     TITLE                              READS  LAST READ
2523154  Live Verify Phase 02 - ...         1      2026-04-26 07:59

$ dandori analytics iterations --by engineer
ENGINEER  AVG ROUND  MAX ROUND  TASKS
(none)    1.00       1          4
```

## Gaps remaining

1. **Phase 04 wiring (P2 / 3d / optional)** — `bugLinkCycle` ticker on `Poller`, JQL search for new Bug issues, bug analytics aggregations + `--bugs` flag, `last_scan_at` persistence
2. **Jira poller daemon command** — Phase 03 logic is wired into `Poller.Poll()` but no CLI command starts the poller in foreground/daemon mode (only `dandori watch` exists, that one polls Claude session logs not Jira)
3. **REFACTOR todos** from each phase: config-driven `causedByLinkTypes`, config-driven `doneCategories` / `activeCategories`, `docs/bug-link.md` user-facing convention guide
4. **Skill events not observed live** — Claude `--print` mode answered the test prompt from training data without invoking the Skill tool. Unit tests cover the parser via fixture session JSONL.

## Files

New:
- `internal/jira/iteration.go` + `_test.go`
- `internal/jira/buglink.go` + `_test.go`
- `internal/jira/poller_iteration_test.go`
- `internal/db/iteration.go` + `_test.go`
- `internal/db/buglink.go` + `_test.go`
- `internal/db/event_analytics.go` + `_test.go`
- `internal/wrapper/tailer_events.go` + `_test.go`
- `internal/wrapper/tailer_recorder_test.go`
- `internal/wrapper/iteration_end_test.go`
- `internal/wrapper/testdata/session-with-tools.jsonl`
- `internal/taskcontext/recorder_test.go`
- `cmd/analytics_events.go`

Modified:
- `internal/jira/models.go` — added `StatusCategoryKey` to `Issue`
- `internal/jira/poller.go` — `LocalDB` + `Recorder` fields, `detectIterations()` end of cycle
- `internal/wrapper/tailer.go` — recorder/runID params, calls `parseLineForEvents`
- `internal/wrapper/wrapper.go` — `emitIterationEndIfApplicable` after run completion
- `internal/taskcontext/context.go` — Recorder param, emits `confluence.read` per page
- `cmd/task_run.go` — pre-creates run row, passes recorder to taskcontext
- `internal/model/event.go` — `LayerSemantic = 3`

## Next

Pick up Phase 04 wiring + jira-poll daemon command (single short feature) — turns
both Phase 03 + 04 from "core verified" into "auto-running in production".
