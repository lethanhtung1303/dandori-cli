---
title: "Phase 01: Lint + Test Delta"
status: done
priority: P0
effort: 6h
completed: 2026-04-19
---

# Phase 01: Lint + Test Delta

> Capture lint errors and test results before/after agent run.
> Foundation for all quality metrics.

---

## Context Links

- [Plan Overview](plan.md)
- [Existing wrapper](../../internal/wrapper/wrapper.go)
- [Run model](../../internal/model/run.go)
- [DB schema](../../internal/db/schema.go)

---

## Overview

**Priority**: P0 (Foundation)
**Status**: Pending
**Effort**: 6h

Extend the wrapper to snapshot lint/test state before execution, re-run after, and compute deltas. Store in new `quality_metrics` table.

---

## Key Insights

1. **JSON output is key**: Both `golangci-lint` and `go test` support JSON output for reliable parsing
2. **Language-agnostic design**: Config specifies lint/test commands; not hardcoded to Go
3. **Fail-safe**: If lint/test capture fails, run continues; quality data marked "unknown"
4. **Timeout critical**: Lint/test snapshots must not block agent execution (5s timeout)

---

## Requirements

### Functional

1. Before `dandori run`, capture:
   - Lint errors/warnings count (via configured linter)
   - Test pass/fail/total count (via configured test runner)
2. After run completes, re-capture same metrics
3. Compute deltas (after - before)
4. Store in `quality_metrics` table linked to run
5. Display in `dandori status --run=X`

### Non-Functional

1. Snapshot timeout: 5 seconds per command
2. No blocking: snapshot failure does not block run
3. Configurable: lint/test commands in config.yaml

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        dandori run                              │
├─────────────────────────────────────────────────────────────────┤
│  1. Create run record                                           │
│  2. Snapshot lint state ──────────────────► LintSnapshot        │
│  3. Snapshot test state ──────────────────► TestSnapshot        │
│  4. Execute agent (existing wrapper)                            │
│  5. On completion:                                              │
│     a. Re-run lint ───────────────────────► LintSnapshot        │
│     b. Re-run tests ──────────────────────► TestSnapshot        │
│     c. Compute deltas                                           │
│     d. Insert quality_metrics                                   │
│  6. Update run status (existing)                                │
└─────────────────────────────────────────────────────────────────┘
```

### Data Flow

```
Config (config.yaml)
    │
    ├── quality.lint_command: "golangci-lint run --json"
    ├── quality.test_command: "go test -json ./..."
    └── quality.timeout: "5s"
            │
            ▼
    ┌───────────────┐     ┌───────────────┐
    │ LintCollector │     │ TestCollector │
    │   Run()       │     │   Run()       │
    │   Parse()     │     │   Parse()     │
    └───────────────┘     └───────────────┘
            │                     │
            ▼                     ▼
    ┌───────────────────────────────────────┐
    │           QualitySnapshot             │
    │  LintErrors, LintWarnings             │
    │  TestsTotal, TestsPassed, TestsFailed │
    └───────────────────────────────────────┘
            │
            ▼
    ┌───────────────────────────────────────┐
    │         quality_metrics table         │
    └───────────────────────────────────────┘
```

---

## Related Code Files

### Files to Create

| File | Purpose |
|------|---------|
| `internal/quality/collector.go` | Lint/test collection logic |
| `internal/quality/lint_parser.go` | Parse golangci-lint JSON output |
| `internal/quality/test_parser.go` | Parse go test -json output |
| `internal/quality/types.go` | QualitySnapshot, QualityMetrics types |
| `internal/quality/collector_test.go` | Unit tests |

### Files to Modify

| File | Change |
|------|--------|
| `internal/db/schema.go` | Add quality_metrics table |
| `internal/db/migrate.go` | Migration for new table |
| `internal/config/config.go` | Add quality config section |
| `internal/wrapper/wrapper.go` | Call quality collector before/after |
| `cmd/status.go` | Display quality metrics |

---

## Implementation Steps

### Step 1: Define Types (30 min)

Create `internal/quality/types.go`:

```go
package quality

type Snapshot struct {
    LintErrors   int
    LintWarnings int
    TestsTotal   int
    TestsPassed  int
    TestsFailed  int
    CapturedAt   time.Time
    Error        string // Non-empty if capture failed
}

type Metrics struct {
    RunID string
    
    // Before
    LintErrorsBefore   int
    LintWarningsBefore int
    TestsTotalBefore   int
    TestsPassedBefore  int
    TestsFailedBefore  int
    
    // After
    LintErrorsAfter   int
    LintWarningsAfter int
    TestsTotalAfter   int
    TestsPassedAfter  int
    TestsFailedAfter  int
    
    // Computed
    LintDelta  int // Negative = improvement
    TestsDelta int // Positive = improvement
}
```

### Step 2: Config Extension (20 min)

Add to `internal/config/config.go`:

```go
type QualityConfig struct {
    Enabled     bool   `yaml:"enabled"`
    LintCommand string `yaml:"lint_command"`
    TestCommand string `yaml:"test_command"`
    Timeout     string `yaml:"timeout"` // e.g., "5s"
}
```

Default config.yaml:

```yaml
quality:
  enabled: true
  lint_command: "golangci-lint run --json --out-format json"
  test_command: "go test -json -count=1 ./..."
  timeout: "5s"
```

### Step 3: Lint Parser (1h)

Create `internal/quality/lint_parser.go`:

```go
// ParseGolangciLint parses golangci-lint JSON output
// Returns (errors, warnings, error)
func ParseGolangciLint(output []byte) (int, int, error) {
    // golangci-lint JSON format:
    // {"Issues": [{"Severity": "error"|"warning", ...}]}
}
```

Handle formats:
- golangci-lint JSON
- Generic: count lines with "error:" or "warning:"

### Step 4: Test Parser (1h)

Create `internal/quality/test_parser.go`:

```go
// ParseGoTestJSON parses go test -json output
// Returns (total, passed, failed, error)
func ParseGoTestJSON(output []byte) (int, int, int, error) {
    // go test -json format: stream of JSON objects
    // {"Action": "pass"|"fail"|"skip", "Test": "TestFoo"}
}
```

### Step 5: Collector (1h)

Create `internal/quality/collector.go`:

```go
type Collector struct {
    cfg     QualityConfig
    timeout time.Duration
}

func (c *Collector) Snapshot(cwd string) *Snapshot {
    // 1. Run lint command with timeout
    // 2. Parse output
    // 3. Run test command with timeout
    // 4. Parse output
    // 5. Return Snapshot
}

func ComputeMetrics(runID string, before, after *Snapshot) *Metrics {
    // Calculate deltas
}
```

### Step 6: Schema Migration (30 min)

Add to `internal/db/schema.go`:

```go
const SchemaSQLV2 = `
CREATE TABLE IF NOT EXISTS quality_metrics (
    run_id TEXT PRIMARY KEY REFERENCES runs(id),
    lint_errors_before INTEGER,
    lint_errors_after INTEGER,
    lint_warnings_before INTEGER,
    lint_warnings_after INTEGER,
    tests_total_before INTEGER,
    tests_passed_before INTEGER,
    tests_failed_before INTEGER,
    tests_total_after INTEGER,
    tests_passed_after INTEGER,
    tests_failed_after INTEGER,
    lint_delta INTEGER,
    tests_delta INTEGER,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_quality_run ON quality_metrics(run_id);
`
```

### Step 7: Wrapper Integration (1h)

Modify `internal/wrapper/wrapper.go`:

```go
func (w *Wrapper) Run(ctx context.Context, args []string) (*model.Run, error) {
    // ... existing setup ...
    
    // NEW: Quality snapshot before
    var qualityBefore *quality.Snapshot
    if w.cfg.Quality.Enabled {
        collector := quality.NewCollector(w.cfg.Quality)
        qualityBefore = collector.Snapshot(cwd)
    }
    
    // ... execute agent (existing) ...
    
    // NEW: Quality snapshot after
    if w.cfg.Quality.Enabled && qualityBefore != nil {
        qualityAfter := collector.Snapshot(cwd)
        metrics := quality.ComputeMetrics(run.ID, qualityBefore, qualityAfter)
        if err := w.db.InsertQualityMetrics(metrics); err != nil {
            slog.Warn("failed to store quality metrics", "error", err)
            // Non-fatal: don't fail the run
        }
    }
    
    // ... existing completion ...
}
```

### Step 8: Status Display (30 min)

Modify `cmd/status.go` to include quality:

```
Run: abc123
Status: done (exit 0)
Duration: 45s
Cost: $0.12

Quality:
  Lint: 5 → 2 (-3 errors)
  Tests: 42/45 → 45/45 (+3 passing)
```

---

## Todo List

- [x] Create `internal/quality/types.go`
- [x] Create `internal/quality/lint_parser.go`
- [x] Create `internal/quality/lint_parser_test.go`
- [x] Create `internal/quality/test_parser.go`
- [x] Create `internal/quality/test_parser_test.go`
- [x] Create `internal/quality/collector.go`
- [x] Create `internal/quality/collector_test.go`
- [x] Update `internal/config/config.go` with QualityConfig
- [x] Update `internal/db/schema.go` with quality_metrics table
- [x] Update `internal/db/migrate.go` for schema v2
- [x] Update `internal/wrapper/wrapper.go` to call collector
- [x] Add `cmd/analytics.go` quality subcommand
- [x] Add `internal/db/quality.go` for DB methods
- [x] Add E2E tests (Group P)

---

## Success Criteria

1. `dandori run -- claude "fix lint"` captures before/after lint counts
2. `dandori status --run=X` shows lint/test deltas
3. Quality capture failure does not block agent run
4. Timeout respected (5s max per snapshot)
5. Works with default Go tooling out of box

---

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| golangci-lint not installed | Medium | Low | Graceful fallback; log warning |
| JSON output format changes | Low | Medium | Version-pinned parsing; fallback to line count |
| Tests take too long | Medium | Medium | Strict timeout; skip test snapshot |
| Non-Go projects | High | Low | Config allows custom commands |

---

## Security Considerations

- Lint/test commands from config, not user input
- No shell expansion; use exec.Command with args
- Output sanitized before storage (no secrets in lint output)

---

## Next Steps

After Phase 01:
- Phase 02: Git metrics (commit count, churn, rework rate)
- Phase 03: Complexity analysis (cyclomatic complexity delta)
