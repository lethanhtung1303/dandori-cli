# 2026-04-30 — G8 Intent Preservation v1

Captured agent intent + decisions + spec linkage from session JSONL. Ships as v0.6.0.

## What shipped

- New events at layer=4: `intent.extracted`, `decision.point`, `agent.reasoning`
- Jira completion comment now includes `h3. Intent` / `h3. Key Decisions` sections (fail-soft: legacy runs unchanged)
- New command: `dandori incident-report --run <id>` and `--task <key>`
- Pure passive parsing (no agent cooperation); env gate `DANDORI_INTENT_DISABLED=1`

## Phase breakdown

| Phase | Tests | Notes |
|-------|-------|-------|
| P1 JSONL walker + extractor | 21 | `internal/intent/` — Walk, Extract, redact, truncate |
| P2 Decision extraction | 15 | Regex heuristics, 5 patterns, follow-up lookahead, cap=5 |
| P3 Spec linkage | 20 | Confluence URL regex, cwd scan, plans/ scan |
| P4 Jira comment extension | 18 | `formatRunCommentWithStore`, `appendG8Sections`, db round-trip |
| P5 incident-report command | 29 | `BuildRunReport`, `BuildTaskReport`, `cmd/incident_report.go` |
| P6 Integration tests + docs | 5 | Full pipeline JSONL→DB→report, fail-soft, cross-run aggregation |

**Total new tests: 108** (all pass, `go test ./...` green)

## Key design decisions

- Fail-soft everywhere: parse errors logged at Warn, run result unaffected
- Decisions capped at 5/run; advisory tag in all output surfaces
- No agent cooperation in v1 — purely passive post-run parsing
- Spec linkage scans README.md, CLAUDE.md, plan.md in cwd + plans/*/plan.md

## Files created

- `internal/intent/extractor.go`, `decisions.go`, `spec_link.go`, `redact.go`, `jsonl_walker.go`, `report.go`
- `internal/intent/integration_test.go` (P6 — 5 cross-boundary tests)
- `internal/db/intent_events.go`, `incident_report_queries.go`
- `cmd/incident_report.go`, `cmd/jira_sync.go` (G8 sections)
- `docs/intent-preservation.md`
