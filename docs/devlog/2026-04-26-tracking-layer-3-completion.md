# 2026-04-26 — Tracking Layer-3 Completion (all 5 phases live)

## Summary

Closed all 5 originally identified Layer-3 tracking gaps from `plans/260425-2100-tracking-layer-3-completion/`.
All TDD (RED → GREEN). Full `go test ./...` green. Live verified against
fooknt.atlassian.net (CLITEST project / space).

## Phases delivered

| # | Phase | Outcome | Live? |
|---|---|---|---|
| 01 | Tool/skill events from JSONL | Tailer parses `tool.use` / `tool.result` / `skill.list` / `skill.invoke` from session log; payload keeps keys + sizes only (no values) | ✅ tool.use + tool.result observed in events table |
| 02 | Confluence read events | `taskcontext.Build` records `confluence.read{page_id, title, char_count, version}` per page fetched | ✅ 1 event for CLITEST-1 with linked test page |
| 03 | Task iteration tracking | `DetectIteration` (workflow-agnostic via `statusCategory.key`) + poller integration + wrapper emits `task.iteration.end` | ⚠️ via integration test (httptest); CLI daemon command not yet wired |
| 05 | Analytics queries | `dandori analytics tools \| context \| iterations` with `--since / --top / --by / --format` flags | ✅ all three return real data, JSON format valid |
| 04 | Bug-link Jira | `ParseDescriptionTags`, `ParseLinkCandidates`, `DetectBugLinks` + `BugLinkResolver` interface + DB resolvers + `bugLinkCycle` ticker on `Poller` + `dandori analytics bugs --by agent\|task` + `dandori jira-poll [--once\|--bug-interval]` daemon command | ✅ CLITEST-61 description-tag bug → `bug.filed` event → analytics returns alpha=1 / TASK=1 |

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

$ # Phase 04: created CLITEST-61 (Bug) with description "caused_by:5be1c1435d3e"
$ dandori jira-poll --once
... INFO msg="bug linked" bug_key=CLITEST-61 run=5be1c1435d3ef76e link_type=description_tag
$ dandori analytics bugs
AGENT           BUGS  LAST FILED
e2e-test-alpha  1     2026-04-26 08:24
$ dandori analytics bugs --by task
TASK       BUGS  LAST FILED
CLITEST-1  1     2026-04-26 08:24
$ dandori jira-poll --once   # second run — dedupe via bug_key
(no new bug.filed event; count remains 1)
```

## Gaps remaining

1. **REFACTOR todos** from each phase: config-driven `causedByLinkTypes`, config-driven `doneCategories` / `activeCategories`, `docs/bug-link.md` user-facing convention guide
2. **"is caused by" link path not exercised live** — fooknt Jira instance lacks the link type. Unit tests cover the path via stubbed link payloads. Description-tag path verified end-to-end.
3. **Skill events not observed live** — Claude `--print` mode answered the test prompt from training data without invoking the Skill tool. Unit tests cover the parser via fixture session JSONL.

## Files

New:
- `internal/jira/iteration.go` + `_test.go`
- `internal/jira/buglink.go` + `_test.go`
- `internal/jira/buglink_search_test.go`
- `internal/jira/poller_iteration_test.go`
- `internal/jira/poller_buglink_test.go`
- `internal/db/iteration.go` + `_test.go`
- `internal/db/buglink.go` + `_test.go`
- `internal/db/event_analytics.go` + `_test.go`
- `internal/db/bug_analytics.go` + `_test.go`
- `internal/wrapper/tailer_events.go` + `_test.go`
- `internal/wrapper/tailer_recorder_test.go`
- `internal/wrapper/iteration_end_test.go`
- `internal/wrapper/testdata/session-with-tools.jsonl`
- `internal/taskcontext/recorder_test.go`
- `cmd/analytics_events.go`
- `cmd/jira_poll.go`

Modified:
- `internal/jira/models.go` — added `StatusCategoryKey`, `Links []IssueLink`, `IssueLinks` parsing on `issueResponse`
- `internal/jira/client.go` — `SearchBugs(jql, max)` for the bug-link cycle
- `internal/jira/buglink.go` — `BugIssue.FromIssue` adapter
- `internal/jira/poller.go` — `LocalDB` + `Recorder` fields, `detectIterations()` per cycle, `bugLinkCycle()` on a separate ticker, `BugLinkCycleOnce()` for `--once` mode
- `internal/wrapper/tailer.go` — recorder/runID params, calls `parseLineForEvents`
- `internal/wrapper/wrapper.go` — `emitIterationEndIfApplicable` after run completion
- `internal/taskcontext/context.go` — Recorder param, emits `confluence.read` per page
- `cmd/task_run.go` — pre-creates run row, passes recorder to taskcontext
- `internal/model/event.go` — `LayerSemantic = 3`

## Next

REFACTOR pass: pull `causedByLinkTypes`, `doneCategories`, `activeCategories` into
config so non-default Jira workflows (and instances missing "is caused by") can be
adapted without code changes. Then `docs/bug-link.md` user-facing convention guide.
