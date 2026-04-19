---
title: "Phase 02: Git Metrics + Rework Detection"
status: done
priority: P1
effort: 4h
completed: 2026-04-19
---

# Phase 02: Git Metrics + Rework Detection

> Capture git diff stats, commit hygiene, and detect rework patterns.
> Measures code churn and re-runs on same task.

---

## Context Links

- [Plan Overview](plan.md)
- [Phase 01: Lint + Test Delta](phase-01-lint-test-delta.md)
- [Existing run model](../../internal/model/run.go) — has git_head_before/after
- [Existing wrapper](../../internal/wrapper/wrapper.go)

---

## Overview

**Priority**: P1
**Status**: Pending (blocked by Phase 01)
**Effort**: 4h

Extend quality metrics with git-derived signals:
- Lines added/removed/changed
- Files changed count
- Commit count and message quality
- Rework detection (multiple runs on same PBI)

---

## Key Insights

1. **git diff --stat** provides lines/files changed — no external tools needed
2. **Commit message quality** can be scored against conventional commits pattern
3. **Rework rate** = runs_per_pbi; high rework suggests agent struggled
4. **Churn** = (lines_added + lines_removed) / task_duration — velocity signal

---

## Requirements

### Functional

1. After run completes, compute:
   - Lines added, removed (from git diff)
   - Files changed count
   - Commits made during run (compare git_head_before → git_head_after)
   - Commit message quality score (0-1)
2. Rework detection:
   - Count runs per jira_issue_key
   - Flag if >2 runs on same PBI
3. Store in quality_metrics table (extend from Phase 01)

### Non-Functional

1. Git operations: <1s total
2. Works in detached HEAD state
3. Works with uncommitted changes (stash scenario)

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                    POST-RUN PROCESSING                          │
├─────────────────────────────────────────────────────────────────┤
│  git_head_before ──────────────────────────────┐                │
│  git_head_after  ──────────────────────────────┤                │
│                                                ▼                │
│                                    ┌───────────────────┐        │
│                                    │   GitAnalyzer     │        │
│                                    │                   │        │
│                                    │ • DiffStats()     │        │
│                                    │ • CommitsBetween()│        │
│                                    │ • ScoreMessages() │        │
│                                    └───────────────────┘        │
│                                                │                │
│                                                ▼                │
│                                    ┌───────────────────┐        │
│                                    │ quality_metrics   │        │
│                                    │ + lines_added     │        │
│                                    │ + lines_removed   │        │
│                                    │ + files_changed   │        │
│                                    │ + commit_count    │        │
│                                    │ + commit_quality  │        │
│                                    └───────────────────┘        │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│                    REWORK DETECTION                             │
├─────────────────────────────────────────────────────────────────┤
│  SELECT COUNT(*) FROM runs WHERE jira_issue_key = ?             │
│  → rework_count (displayed in analytics)                        │
└─────────────────────────────────────────────────────────────────┘
```

---

## Related Code Files

### Files to Create

| File | Purpose |
|------|---------|
| `internal/quality/git_analyzer.go` | Git diff stats, commit analysis |
| `internal/quality/commit_scorer.go` | Conventional commits scoring |
| `internal/quality/git_analyzer_test.go` | Unit tests |

### Files to Modify

| File | Change |
|------|--------|
| `internal/quality/types.go` | Add git metrics fields |
| `internal/db/schema.go` | Add git columns to quality_metrics |
| `internal/wrapper/wrapper.go` | Call git analyzer post-run |
| `internal/analytics/types.go` | Add rework_count to AgentComparison |
| `internal/analytics/queries.go` | Query rework stats |

---

## Implementation Steps

### Step 1: Git Analyzer (1.5h)

Create `internal/quality/git_analyzer.go`:

```go
package quality

import (
    "os/exec"
    "strconv"
    "strings"
)

type GitStats struct {
    LinesAdded   int
    LinesRemoved int
    FilesChanged int
    CommitCount  int
    CommitMsgs   []string
}

type GitAnalyzer struct {
    cwd string
}

func NewGitAnalyzer(cwd string) *GitAnalyzer {
    return &GitAnalyzer{cwd: cwd}
}

// DiffStats returns lines added/removed between two commits
func (g *GitAnalyzer) DiffStats(before, after string) (*GitStats, error) {
    if before == "" || after == "" || before == after {
        return &GitStats{}, nil
    }
    
    // git diff --stat --numstat before..after
    cmd := exec.Command("git", "diff", "--numstat", before+".."+after)
    cmd.Dir = g.cwd
    out, err := cmd.Output()
    if err != nil {
        return nil, err
    }
    
    stats := &GitStats{}
    for _, line := range strings.Split(string(out), "\n") {
        fields := strings.Fields(line)
        if len(fields) >= 3 {
            added, _ := strconv.Atoi(fields[0])
            removed, _ := strconv.Atoi(fields[1])
            stats.LinesAdded += added
            stats.LinesRemoved += removed
            stats.FilesChanged++
        }
    }
    
    // Count commits
    cmd = exec.Command("git", "rev-list", "--count", before+".."+after)
    cmd.Dir = g.cwd
    out, err = cmd.Output()
    if err == nil {
        stats.CommitCount, _ = strconv.Atoi(strings.TrimSpace(string(out)))
    }
    
    // Get commit messages
    if stats.CommitCount > 0 {
        cmd = exec.Command("git", "log", "--format=%s", before+".."+after)
        cmd.Dir = g.cwd
        out, _ = cmd.Output()
        stats.CommitMsgs = strings.Split(strings.TrimSpace(string(out)), "\n")
    }
    
    return stats, nil
}
```

### Step 2: Commit Scorer (1h)

Create `internal/quality/commit_scorer.go`:

```go
package quality

import (
    "regexp"
    "strings"
)

// Conventional commit pattern: type(scope): description
var conventionalPattern = regexp.MustCompile(`^(feat|fix|docs|style|refactor|test|chore|perf|ci|build|revert)(\(.+\))?: .{10,}`)

// ScoreCommitMessages returns 0-1 quality score
func ScoreCommitMessages(msgs []string) float64 {
    if len(msgs) == 0 {
        return 0
    }
    
    var score float64
    for _, msg := range msgs {
        score += scoreMessage(msg)
    }
    return score / float64(len(msgs))
}

func scoreMessage(msg string) float64 {
    var score float64
    
    // Conventional commit format (+0.4)
    if conventionalPattern.MatchString(msg) {
        score += 0.4
    }
    
    // Reasonable length: 10-72 chars (+0.2)
    if len(msg) >= 10 && len(msg) <= 72 {
        score += 0.2
    }
    
    // Starts with capital after prefix (+0.1)
    parts := strings.SplitN(msg, ": ", 2)
    if len(parts) == 2 && len(parts[1]) > 0 {
        if parts[1][0] >= 'A' && parts[1][0] <= 'Z' {
            score += 0.1
        }
    }
    
    // No "WIP", "fixup", "squash" (+0.2)
    lower := strings.ToLower(msg)
    if !strings.Contains(lower, "wip") &&
       !strings.Contains(lower, "fixup") &&
       !strings.Contains(lower, "squash") {
        score += 0.2
    }
    
    // References issue (+0.1)
    if strings.Contains(msg, "#") || 
       regexp.MustCompile(`[A-Z]+-\d+`).MatchString(msg) {
        score += 0.1
    }
    
    return score
}
```

### Step 3: Extend Types (20 min)

Update `internal/quality/types.go`:

```go
type Metrics struct {
    // ... existing fields from Phase 01 ...
    
    // Git metrics (Phase 02)
    LinesAdded          int
    LinesRemoved        int
    FilesChanged        int
    CommitCount         int
    CommitMsgQuality    float64 // 0-1
}
```

### Step 4: Schema Update (20 min)

Update quality_metrics table:

```sql
-- Add columns (migration)
ALTER TABLE quality_metrics ADD COLUMN lines_added INTEGER;
ALTER TABLE quality_metrics ADD COLUMN lines_removed INTEGER;
ALTER TABLE quality_metrics ADD COLUMN files_changed INTEGER;
ALTER TABLE quality_metrics ADD COLUMN commit_count INTEGER;
ALTER TABLE quality_metrics ADD COLUMN commit_msg_quality REAL;
```

### Step 5: Rework Query (30 min)

Add to `internal/analytics/queries.go`:

```go
func (q *Querier) AgentReworkStats(ctx context.Context, f Filters) ([]AgentRework, error) {
    query := `
        SELECT
            agent_name,
            COUNT(DISTINCT jira_issue_key) as tasks,
            COUNT(*) as total_runs,
            ROUND(COUNT(*)::numeric / NULLIF(COUNT(DISTINCT jira_issue_key), 0), 2) as runs_per_task,
            COUNT(*) FILTER (WHERE rework_count > 2) as high_rework_tasks
        FROM (
            SELECT 
                agent_name,
                jira_issue_key,
                COUNT(*) OVER (PARTITION BY jira_issue_key) as rework_count
            FROM runs
            WHERE jira_issue_key IS NOT NULL
        ) sub
        WHERE 1=1
    `
    // ... filters and execution ...
}
```

### Step 6: Wrapper Integration (30 min)

Update `internal/wrapper/wrapper.go` post-run:

```go
// After agent completes, before storing quality metrics
if run.GitHeadBefore.Valid && run.GitHeadAfter.Valid {
    analyzer := quality.NewGitAnalyzer(cwd)
    stats, err := analyzer.DiffStats(
        run.GitHeadBefore.String,
        run.GitHeadAfter.String,
    )
    if err == nil {
        metrics.LinesAdded = stats.LinesAdded
        metrics.LinesRemoved = stats.LinesRemoved
        metrics.FilesChanged = stats.FilesChanged
        metrics.CommitCount = stats.CommitCount
        metrics.CommitMsgQuality = quality.ScoreCommitMessages(stats.CommitMsgs)
    }
}
```

---

## Todo List

- [x] Create `internal/quality/git_analyzer.go`
- [x] Create `internal/quality/git_analyzer_test.go`
- [x] Create `internal/quality/commit_scorer.go`
- [x] Update `internal/quality/types.go` with git fields
- [x] Add git columns to quality_metrics schema
- [x] Update wrapper to call git analyzer
- [x] Update db/quality.go for git metrics
- [x] Update analytics quality command with git stats

---

## Success Criteria

1. `dandori status --run=X` shows lines added/removed, commit count
2. `dandori analytics rework` shows runs_per_task by agent
3. Commit quality score reflects conventional commits adherence
4. Works when no commits made (returns zeros)

---

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Shallow clone lacks history | Medium | Low | Detect and warn; skip git metrics |
| Agent doesn't commit | Medium | Low | Zero values valid; lint/test still useful |
| Rebase rewrites commits | Low | Low | Use HEAD references; accept variance |

---

## Next Steps

After Phase 02:
- Phase 03: Complexity analysis (optional, P2)
- Phase 04: Quality analytics and comparison views
