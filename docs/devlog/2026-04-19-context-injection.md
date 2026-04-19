# 2026-04-19 — Context Injection & Enhanced Tracking

## Summary

Implemented `dandori task run` — the recommended workflow that auto-fetches Jira+Confluence context and injects it into the agent prompt.

## What Changed

### New: Task Run with Context (`cmd/task_run.go`)
- Fetches Jira issue (summary, description, AC)
- Extracts Confluence links from description via regex
- Fetches linked Confluence page content
- Generates markdown context file
- Injects context into Claude's `-p` prompt
- Auto-transitions Jira (To Do → In Progress → Done)
- Posts comprehensive completion comment

### New: Enhanced Jira Completion Comment
- Run statistics: agent, duration, cost, tokens, model
- Git HEAD before → after comparison
- Files changed during the run
- Commits made during the run
- Acceptance Criteria extracted from task (for manual verification)
- Output location with `conf-write` command

### New: Cost Calculation
Model-specific pricing (per 1M tokens):
| Model | Input | Output | Cache Write | Cache Read |
|-------|-------|--------|-------------|------------|
| Sonnet 4.6 | $3.00 | $15.00 | $3.75 | $0.30 |
| Opus 4.5/4.6 | $15.00 | $75.00 | $18.75 | $1.50 |
| Haiku 4.5 | $0.80 | $4.00 | $1.00 | $0.08 |

### Bug Fixes
- Context injection now prepends to user's `-p` prompt (not replaces)
- Git changes track only commits made during the run (before/after comparison)

## Files Added/Modified
- `internal/taskcontext/context.go` — Jira+Confluence fetcher
- `cmd/task_run.go` — New command
- `internal/wrapper/wrapper.go` — Cost calculation
- `scripts/create_diverse_fixtures.go` — Test fixtures

## E2E Tests
Added Groups N (8 tests) and O (4 tests):
- Context injection with various task types
- Multi-link Confluence extraction
- Code blocks in descriptions
- Bug vs Story vs Task handling

Total: 66 E2E tests across 15 groups (A-O).

## Release
Published as v0.2.0 via goreleaser.
