---
title: "Agent Quality Comparison"
description: "Code quality metrics for comparing agent effectiveness across teams"
status: pending
priority: P1
effort: 16h
branch: main
tags: [analytics, quality, metrics, comparison]
created: 2026-04-19
---

# Agent Quality Comparison — Implementation Plan

> Enable PO/PDM to answer: "Does Team A's agent write better code than Team B?"
> Objective measurement of agent code quality beyond exit codes.

---

## Problem Statement

**Current state**: dandori-cli tracks run metadata (duration, exit_code, tokens, cost, story_points). Success is defined as `exit_code = 0`. This measures "did it finish?" not "was the code good?"

**Gap**: No code quality signals. Two agents with 100% success_rate may produce vastly different code quality.

**Business need**: PO/PDM needs data-driven agent assignment. QA needs quality trend visibility.

---

## Metrics Taxonomy

| Metric | Data Source | Automation | Value | Phase |
|--------|-------------|------------|-------|-------|
| **Lint delta** | Pre/post linter output | Full | High | 01 |
| **Test delta** | Pre/post test results | Full | High | 01 |
| **Commit hygiene** | Git commit message analysis | Full | Medium | 02 |
| **Churn rate** | Git diff stats per run | Full | Medium | 02 |
| **Complexity delta** | Static analysis (gocyclo, etc.) | Full | Medium | 03 |
| **PR review time** | Jira/GitHub API (future) | Semi | High | Future |
| **Bug rate** | Jira defect linkage | Manual | High | Future |
| **Rework rate** | Re-runs on same PBI | Full | High | 02 |

**Dropped** (YAGNI): Security scan, documentation coverage, mutation testing — too complex for initial release.

---

## Architecture

```
                          ┌─────────────────────────────────────┐
                          │           RUN LIFECYCLE             │
                          └─────────────────────────────────────┘
                                          │
        ┌───────────────────────────────────────────────────────────┐
        │                     BEFORE RUN                            │
        │  • Snapshot lint state (golangci-lint --json)             │
        │  • Snapshot test state (go test -json -count=0 -list .)   │
        │  • Record git HEAD                                        │
        └───────────────────────────────────────────────────────────┘
                                          │
                                          ▼
        ┌───────────────────────────────────────────────────────────┐
        │                     AGENT EXECUTION                       │
        │  • dandori run -- claude "fix the bug"                    │
        │  • (existing wrapper captures duration, tokens, cost)     │
        └───────────────────────────────────────────────────────────┘
                                          │
                                          ▼
        ┌───────────────────────────────────────────────────────────┐
        │                     AFTER RUN                             │
        │  • Re-run lint, diff against snapshot                     │
        │  • Re-run tests, diff against snapshot                    │
        │  • Compute git diff stats (lines added/removed/changed)   │
        │  • Store quality_metrics in runs table or separate table  │
        └───────────────────────────────────────────────────────────┘
                                          │
                                          ▼
        ┌───────────────────────────────────────────────────────────┐
        │                     ANALYTICS                             │
        │  • Quality score per run                                  │
        │  • Aggregate by agent, team, task type                    │
        │  • Trend over time                                        │
        │  • Compare agents on quality dimensions                   │
        └───────────────────────────────────────────────────────────┘
```

---

## Data Model Extension

```sql
-- New table: quality_metrics (per run)
CREATE TABLE quality_metrics (
    run_id TEXT PRIMARY KEY REFERENCES runs(id),
    
    -- Lint metrics
    lint_errors_before INTEGER,
    lint_errors_after INTEGER,
    lint_warnings_before INTEGER,
    lint_warnings_after INTEGER,
    lint_delta INTEGER GENERATED ALWAYS AS (lint_errors_after - lint_errors_before) STORED,
    
    -- Test metrics
    tests_total_before INTEGER,
    tests_passed_before INTEGER,
    tests_failed_before INTEGER,
    tests_total_after INTEGER,
    tests_passed_after INTEGER,
    tests_failed_after INTEGER,
    tests_delta INTEGER GENERATED ALWAYS AS (tests_passed_after - tests_passed_before) STORED,
    
    -- Git metrics
    lines_added INTEGER,
    lines_removed INTEGER,
    files_changed INTEGER,
    
    -- Computed (Phase 02)
    commit_count INTEGER,
    commit_msg_quality_score REAL,  -- 0-1 based on conventional commits
    
    -- Computed (Phase 03)
    complexity_before REAL,
    complexity_after REAL,
    complexity_delta REAL,
    
    -- Aggregate quality score (weighted composite)
    quality_score REAL,
    
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_quality_run ON quality_metrics(run_id);
```

---

## Phases

| Phase | Name | Effort | Depends | Priority |
|-------|------|--------|---------|----------|
| 01 | [Lint + Test Delta](phase-01-lint-test-delta.md) | 6h | — | P0 |
| 02 | [Git Metrics + Rework](phase-02-git-metrics-rework.md) | 4h | 01 | P1 |
| 03 | [Complexity Analysis](phase-03-complexity-analysis.md) | 3h | 01 | P2 |
| 04 | [Quality Analytics](phase-04-quality-analytics.md) | 3h | 01-03 | P1 |

---

## Success Criteria

1. **Measurable**: `dandori analytics quality --agent=X` shows quality scores
2. **Comparable**: `dandori analytics compare --agents=X,Y --metric=quality` works
3. **Actionable**: Dashboard displays quality trends alongside cost/duration
4. **Offline**: All metrics computed locally, no server required
5. **Non-invasive**: Existing runs without quality data display "N/A"

---

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Lint/test output parsing breaks | Medium | High | Use JSON output modes; fallback to "unknown" |
| Metrics collection slows run startup | Low | Medium | Async snapshot; timeout (5s max) |
| Different projects need different linters | High | Medium | Config-driven linter command |
| Quality score weighting is subjective | Medium | Low | Expose raw metrics; make score optional |

---

## Backwards Compatibility

- New `quality_metrics` table; existing `runs` table unchanged
- Runs without quality data: queries return NULL/N/A
- No migration of historical data (can't reconstruct lint/test state)
- Future: optional backfill via `dandori quality scan --run-id=X` if code unchanged

---

## Test Matrix

| Category | Test |
|----------|------|
| Unit | Lint output parser (golangci-lint JSON) |
| Unit | Test output parser (go test -json) |
| Unit | Git diff stats extraction |
| Unit | Quality score calculation |
| Integration | Full run with quality capture |
| Integration | Analytics queries with quality filters |
| E2E | Dashboard quality comparison view |

---

## References

- [Codacy: 8 Code Quality Metrics](https://blog.codacy.com/code-quality-metrics)
- [DORA vs SPACE Framework 2026](https://reintech.io/blog/dora-metrics-vs-space-framework-developer-productivity-2026)
- [Swarmia: Comparing Productivity Frameworks](https://www.swarmia.com/blog/comparing-developer-productivity-frameworks/)
- [Git Hooks for Code Quality 2025](https://dev.to/arasosman/git-hooks-for-automated-code-quality-checks-guide-2025-372f)
