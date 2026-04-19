---
title: "Phase 04: Quality Analytics"
status: pending
priority: P1
effort: 3h
---

# Phase 04: Quality Analytics

> Aggregate quality metrics for comparison and trending.
> Enables "Which agent writes better code?" analysis.

---

## Context Links

- [Plan Overview](plan.md)
- [Phase 01-03](phase-01-lint-test-delta.md) — Quality data sources
- [Existing analytics](../../internal/analytics/queries.go)
- [Existing types](../../internal/analytics/types.go)

---

## Overview

**Priority**: P1
**Status**: Pending (blocked by Phase 01)
**Effort**: 3h

Build analytics queries and CLI commands for quality comparison. Extend dashboard with quality views.

---

## Key Insights

1. **Composite quality score** enables single-number comparison
2. **Per-dimension breakdowns** allow nuanced analysis
3. **Task-type correlation** reveals which agents suit which work
4. **Trend over time** shows if agents are improving

---

## Requirements

### Functional

1. **CLI Commands**:
   - `dandori analytics quality` — Quality summary by agent
   - `dandori analytics compare --agents=A,B --metric=quality`
   - `dandori analytics quality --task-type=Bug`
2. **Quality Score**: Weighted composite (configurable weights)
3. **Dashboard**: Quality tab with comparison charts
4. **Export**: JSON/CSV for external analysis

### Non-Functional

1. Queries performant on 10k+ runs
2. NULL handling for runs without quality data
3. Consistent with existing analytics patterns

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                    QUALITY ANALYTICS                            │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │                    RAW METRICS                           │   │
│  │  • lint_delta per run                                    │   │
│  │  • tests_delta per run                                   │   │
│  │  • complexity_delta per run                              │   │
│  │  • commit_msg_quality per run                            │   │
│  │  • rework_count per PBI                                  │   │
│  └─────────────────────────────────────────────────────────┘   │
│                            │                                    │
│                            ▼                                    │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │                 COMPOSITE SCORE                          │   │
│  │                                                          │   │
│  │  quality_score = w1*lint + w2*tests + w3*complexity      │   │
│  │                  + w4*commits + w5*(1/rework)            │   │
│  │                                                          │   │
│  │  Default weights: lint=0.25, tests=0.30, complexity=0.15 │   │
│  │                   commits=0.10, rework=0.20              │   │
│  └─────────────────────────────────────────────────────────┘   │
│                            │                                    │
│                            ▼                                    │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │                   AGGREGATIONS                           │   │
│  │  • By agent                                              │   │
│  │  • By team                                               │   │
│  │  • By task type (Bug/Story/Task)                         │   │
│  │  • By time period                                        │   │
│  │  • Head-to-head comparison                               │   │
│  └─────────────────────────────────────────────────────────┘   │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

---

## Quality Score Calculation

```go
// NormalizeLintDelta converts lint delta to 0-1 score
// Negative delta (improvement) = higher score
func NormalizeLintDelta(delta int) float64 {
    // -10 or better = 1.0, +10 or worse = 0.0
    normalized := float64(-delta) / 10.0
    return clamp(0.5 + normalized*0.5, 0, 1)
}

// NormalizeTestsDelta converts test delta to 0-1 score
// Positive delta (more passing) = higher score
func NormalizeTestsDelta(delta int) float64 {
    // +10 or better = 1.0, -10 or worse = 0.0
    normalized := float64(delta) / 10.0
    return clamp(0.5 + normalized*0.5, 0, 1)
}

// NormalizeComplexityDelta converts complexity delta to 0-1 score
// Negative delta (simpler) = higher score
func NormalizeComplexityDelta(delta float64) float64 {
    // -5 or better = 1.0, +5 or worse = 0.0
    normalized := -delta / 5.0
    return clamp(0.5 + normalized*0.5, 0, 1)
}

// ComputeQualityScore calculates weighted composite
func ComputeQualityScore(m *QualityMetrics, w Weights) float64 {
    lint := NormalizeLintDelta(m.LintDelta)
    tests := NormalizeTestsDelta(m.TestsDelta)
    complexity := NormalizeComplexityDelta(m.ComplexityDelta)
    commits := m.CommitMsgQuality
    rework := 1.0 / float64(max(m.ReworkCount, 1)) // Inverse: less rework = better
    
    return w.Lint*lint + 
           w.Tests*tests + 
           w.Complexity*complexity + 
           w.Commits*commits + 
           w.Rework*rework
}
```

---

## Related Code Files

### Files to Create

| File | Purpose |
|------|---------|
| `internal/analytics/quality_types.go` | Quality analytics types |
| `internal/analytics/quality_queries.go` | Quality aggregation queries |
| `internal/analytics/quality_score.go` | Score calculation |
| `cmd/analytics_quality.go` | CLI subcommand |

### Files to Modify

| File | Change |
|------|--------|
| `cmd/analytics.go` | Add quality subcommand |
| `internal/server/routes_analytics.go` | Add quality endpoints |
| `internal/analytics/export.go` | Add quality export |

---

## Implementation Steps

### Step 1: Quality Types (30 min)

Create `internal/analytics/quality_types.go`:

```go
package analytics

type QualityWeights struct {
    Lint       float64 `yaml:"lint"`
    Tests      float64 `yaml:"tests"`
    Complexity float64 `yaml:"complexity"`
    Commits    float64 `yaml:"commits"`
    Rework     float64 `yaml:"rework"`
}

var DefaultWeights = QualityWeights{
    Lint:       0.25,
    Tests:      0.30,
    Complexity: 0.15,
    Commits:    0.10,
    Rework:     0.20,
}

type AgentQuality struct {
    AgentName      string  `json:"agent_name"`
    RunCount       int     `json:"run_count"`
    QualityScore   float64 `json:"quality_score"`
    AvgLintDelta   float64 `json:"avg_lint_delta"`
    AvgTestsDelta  float64 `json:"avg_tests_delta"`
    AvgComplexity  float64 `json:"avg_complexity_delta"`
    AvgCommitScore float64 `json:"avg_commit_score"`
    AvgReworkCount float64 `json:"avg_rework_count"`
}

type QualityByTaskType struct {
    TaskType     string  `json:"task_type"`
    AgentName    string  `json:"agent_name"`
    QualityScore float64 `json:"quality_score"`
    RunCount     int     `json:"run_count"`
}

type QualityTrend struct {
    PeriodStart  time.Time `json:"period_start"`
    AgentName    string    `json:"agent_name"`
    QualityScore float64   `json:"quality_score"`
    RunCount     int       `json:"run_count"`
}
```

### Step 2: Quality Queries (1h)

Create `internal/analytics/quality_queries.go`:

```go
package analytics

func (q *Querier) AgentQualityStats(ctx context.Context, f Filters) ([]AgentQuality, error) {
    query := `
        WITH rework AS (
            SELECT 
                jira_issue_key,
                COUNT(*) as rework_count
            FROM runs
            WHERE jira_issue_key IS NOT NULL
            GROUP BY jira_issue_key
        )
        SELECT
            r.agent_name,
            COUNT(*) as run_count,
            COALESCE(AVG(qm.lint_delta), 0) as avg_lint_delta,
            COALESCE(AVG(qm.tests_delta), 0) as avg_tests_delta,
            COALESCE(AVG(qm.complexity_delta), 0) as avg_complexity_delta,
            COALESCE(AVG(qm.commit_msg_quality), 0) as avg_commit_score,
            COALESCE(AVG(rw.rework_count), 1) as avg_rework_count
        FROM runs r
        LEFT JOIN quality_metrics qm ON r.id = qm.run_id
        LEFT JOIN rework rw ON r.jira_issue_key = rw.jira_issue_key
        WHERE 1=1
    `
    // ... filters and execution ...
    
    // Post-process: compute quality_score
    for i := range results {
        results[i].QualityScore = computeQualityScore(results[i], DefaultWeights)
    }
    
    return results, nil
}

func (q *Querier) QualityByTaskType(ctx context.Context, f Filters) ([]QualityByTaskType, error) {
    query := `
        SELECT
            jt.issue_type,
            r.agent_name,
            COUNT(*) as run_count,
            COALESCE(AVG(qm.lint_delta), 0) as avg_lint_delta,
            COALESCE(AVG(qm.tests_delta), 0) as avg_tests_delta
        FROM runs r
        JOIN jira_tasks jt ON r.jira_issue_key = jt.issue_key
        LEFT JOIN quality_metrics qm ON r.id = qm.run_id
        WHERE jt.issue_type IS NOT NULL
        GROUP BY jt.issue_type, r.agent_name
        ORDER BY jt.issue_type, run_count DESC
    `
    // ...
}

func (q *Querier) QualityTrend(ctx context.Context, agent string, period string, depth int) ([]QualityTrend, error) {
    // Similar to CostTrend but for quality metrics
}
```

### Step 3: CLI Command (45 min)

Create `cmd/analytics_quality.go`:

```go
package cmd

var analyticsQualityCmd = &cobra.Command{
    Use:   "quality",
    Short: "Agent quality metrics",
    RunE: func(cmd *cobra.Command, args []string) error {
        // Parse filters
        f := analytics.Filters{
            Agent:    agentFlag,
            SprintID: sprintFlag,
            From:     analytics.ParseTimeFilter(fromFlag),
            To:       analytics.ParseTimeFilter(toFlag),
        }
        
        querier := analytics.NewQuerier(pool)
        stats, err := querier.AgentQualityStats(ctx, f)
        if err != nil {
            return err
        }
        
        // Output
        if outputFormat == "json" {
            return json.NewEncoder(os.Stdout).Encode(stats)
        }
        
        // Table format
        w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
        fmt.Fprintln(w, "AGENT\tRUNS\tQUALITY\tLINT Δ\tTESTS Δ\tCOMPLEXITY Δ")
        for _, s := range stats {
            fmt.Fprintf(w, "%s\t%d\t%.2f\t%+.1f\t%+.1f\t%+.1f\n",
                s.AgentName, s.RunCount, s.QualityScore,
                s.AvgLintDelta, s.AvgTestsDelta, s.AvgComplexity)
        }
        return w.Flush()
    },
}
```

### Step 4: Server Endpoints (30 min)

Add to `internal/server/routes_analytics.go`:

```go
func (s *Server) handleQualityStats(w http.ResponseWriter, r *http.Request) {
    f := parseFilters(r)
    stats, err := s.querier.AgentQualityStats(r.Context(), f)
    if err != nil {
        http.Error(w, err.Error(), 500)
        return
    }
    json.NewEncoder(w).Encode(stats)
}

func (s *Server) handleQualityByTaskType(w http.ResponseWriter, r *http.Request) {
    // ...
}

func (s *Server) handleQualityTrend(w http.ResponseWriter, r *http.Request) {
    // ...
}

// Register routes
r.Get("/api/analytics/quality", s.handleQualityStats)
r.Get("/api/analytics/quality/by-task-type", s.handleQualityByTaskType)
r.Get("/api/analytics/quality/trend", s.handleQualityTrend)
```

### Step 5: Tests (30 min)

```go
func TestAgentQualityStats(t *testing.T) {
    // Insert runs with quality metrics
    // Query and verify aggregations
}

func TestQualityScoreCalculation(t *testing.T) {
    tests := []struct {
        name     string
        metrics  QualityMetrics
        expected float64
    }{
        {
            name: "perfect run",
            metrics: QualityMetrics{
                LintDelta:       -5,  // Fixed 5 lint errors
                TestsDelta:      3,   // Added 3 passing tests
                ComplexityDelta: -2,  // Reduced complexity
                CommitMsgQuality: 0.9,
                ReworkCount:     1,
            },
            expected: 0.85, // Approximately
        },
        // More cases...
    }
}
```

---

## Todo List

- [ ] Create `internal/analytics/quality_types.go`
- [ ] Create `internal/analytics/quality_score.go`
- [ ] Create `internal/analytics/quality_queries.go`
- [ ] Create `internal/analytics/quality_queries_test.go`
- [ ] Create `cmd/analytics_quality.go`
- [ ] Add server endpoints
- [ ] Update analytics export for quality
- [ ] Add dashboard quality view (future)

---

## CLI Examples

```bash
# Quality summary by agent
$ dandori analytics quality
AGENT       RUNS  QUALITY  LINT Δ  TESTS Δ  COMPLEXITY Δ
claude-1    45    0.82     -2.3    +1.5     -0.8
claude-2    38    0.71     -0.5    +0.2     +1.2
codex-1     22    0.65     +1.1    -0.3     +2.1

# Compare two agents
$ dandori analytics compare --agents=claude-1,claude-2 --metric=quality
Metric: Quality Score
claude-1: 0.82 (+15% vs claude-2)
claude-2: 0.71

# Quality by task type
$ dandori analytics quality --by-task-type
TYPE   BEST AGENT   QUALITY
Bug    claude-1     0.88
Story  claude-2     0.75
Task   claude-1     0.80

# Export for spreadsheet
$ dandori analytics quality --output=csv > quality-report.csv
```

---

## Success Criteria

1. `dandori analytics quality` shows per-agent quality scores
2. Scores consistent and reproducible
3. Works with partial data (some runs missing quality metrics)
4. Export to JSON/CSV works

---

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Quality score weights controversial | High | Low | Configurable; expose raw metrics |
| Sparse data skews averages | Medium | Medium | Show run count; require minimum |
| Performance on large datasets | Low | Medium | Indexed queries; pagination |

---

## Dashboard Wireframe (Future)

```
┌─────────────────────────────────────────────────────────────────┐
│  Quality Comparison                        Sprint: Sprint 5 ▼   │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Agent Quality Scores              Quality by Task Type         │
│  ┌─────────────────────────┐      ┌─────────────────────────┐  │
│  │ claude-1  ████████░░ 82%│      │ Bug:   claude-1 best    │  │
│  │ claude-2  ███████░░░ 71%│      │ Story: claude-2 best    │  │
│  │ codex-1   ██████░░░░ 65%│      │ Task:  claude-1 best    │  │
│  └─────────────────────────┘      └─────────────────────────┘  │
│                                                                 │
│  Quality Trend (Last 30 Days)                                   │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │ 1.0 ┤                                                    │   │
│  │     │    ╱──────╲     ╱───────────                       │   │
│  │ 0.5 ┤───╱        ╲───╱                                   │   │
│  │     │                                                    │   │
│  │ 0.0 ┼────────────────────────────────────────────────    │   │
│  │       W1    W2    W3    W4                               │   │
│  └─────────────────────────────────────────────────────────┘   │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

---

## Next Steps

After Phase 04:
- Dashboard integration (HTMX views)
- Alert thresholds (notify if quality drops)
- Agent recommendation based on task type + quality history
