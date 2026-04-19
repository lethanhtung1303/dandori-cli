---
title: "Phase 03: Complexity Analysis"
status: pending
priority: P2
effort: 3h
---

# Phase 03: Complexity Analysis

> Measure cyclomatic complexity delta before/after agent run.
> Advanced metric for code maintainability.

---

## Context Links

- [Plan Overview](plan.md)
- [Phase 01: Lint + Test Delta](phase-01-lint-test-delta.md)
- [Phase 02: Git Metrics](phase-02-git-metrics-rework.md)

---

## Overview

**Priority**: P2 (Optional enhancement)
**Status**: Pending (blocked by Phase 01)
**Effort**: 3h

Add cyclomatic complexity measurement to quality metrics. Answers: "Did the agent make the code more or less complex?"

---

## Key Insights

1. **gocyclo** is the standard Go tool for cyclomatic complexity
2. **Complexity increase** is not always bad (new features add code)
3. **Normalize by lines**: complexity_per_line is more meaningful
4. **High complexity delta** on bug fixes = code smell (should simplify, not complicate)

---

## Requirements

### Functional

1. Before run: measure average cyclomatic complexity of changed files
2. After run: re-measure same files
3. Compute:
   - Total complexity before/after
   - Complexity per file
   - Complexity delta
4. Store in quality_metrics

### Non-Functional

1. Tool optional: graceful fallback if gocyclo not installed
2. Scope to changed files only (performance)
3. Support language-specific tools via config

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                  COMPLEXITY MEASUREMENT                         │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  BEFORE RUN:                                                    │
│  └─ gocyclo -over 0 -avg ./...                                  │
│     └─ Parse: avg complexity, max complexity, total             │
│                                                                 │
│  AFTER RUN:                                                     │
│  └─ gocyclo -over 0 -avg ./...                                  │
│     └─ Parse: same metrics                                      │
│                                                                 │
│  DELTA:                                                         │
│  └─ complexity_delta = after_avg - before_avg                   │
│  └─ Negative = improvement (simpler code)                       │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

---

## Related Code Files

### Files to Create

| File | Purpose |
|------|---------|
| `internal/quality/complexity.go` | Complexity measurement |
| `internal/quality/complexity_test.go` | Unit tests |

### Files to Modify

| File | Change |
|------|--------|
| `internal/quality/types.go` | Add complexity fields |
| `internal/quality/collector.go` | Call complexity analyzer |
| `internal/config/config.go` | Add complexity_command config |
| `internal/db/schema.go` | Add complexity columns |

---

## Implementation Steps

### Step 1: Complexity Analyzer (1.5h)

Create `internal/quality/complexity.go`:

```go
package quality

import (
    "os/exec"
    "regexp"
    "strconv"
    "strings"
)

type ComplexityStats struct {
    Average   float64
    Max       int
    Total     int
    FileCount int
    Error     string
}

// MeasureComplexity runs gocyclo and parses output
func MeasureComplexity(cwd, command string) *ComplexityStats {
    if command == "" {
        command = "gocyclo -over 0 -avg ./..."
    }
    
    args := strings.Fields(command)
    cmd := exec.Command(args[0], args[1:]...)
    cmd.Dir = cwd
    
    out, err := cmd.CombinedOutput()
    if err != nil {
        // gocyclo exits non-zero if complexity > threshold
        // Still parse output
        if len(out) == 0 {
            return &ComplexityStats{Error: err.Error()}
        }
    }
    
    return parseGocycloOutput(string(out))
}

// parseGocycloOutput parses gocyclo output
// Format: "N funcname path/file.go:line:col"
// Last line: "Average: X.XX"
func parseGocycloOutput(output string) *ComplexityStats {
    stats := &ComplexityStats{}
    lines := strings.Split(strings.TrimSpace(output), "\n")
    
    avgPattern := regexp.MustCompile(`Average:\s+([\d.]+)`)
    funcPattern := regexp.MustCompile(`^(\d+)\s+`)
    
    for _, line := range lines {
        if m := avgPattern.FindStringSubmatch(line); len(m) > 1 {
            stats.Average, _ = strconv.ParseFloat(m[1], 64)
            continue
        }
        
        if m := funcPattern.FindStringSubmatch(line); len(m) > 1 {
            complexity, _ := strconv.Atoi(m[1])
            stats.Total += complexity
            stats.FileCount++
            if complexity > stats.Max {
                stats.Max = complexity
            }
        }
    }
    
    return stats
}
```

### Step 2: Extend Types (15 min)

Update `internal/quality/types.go`:

```go
type Snapshot struct {
    // ... existing lint/test fields ...
    
    // Complexity (Phase 03)
    ComplexityAvg float64
    ComplexityMax int
}

type Metrics struct {
    // ... existing fields ...
    
    // Complexity (Phase 03)
    ComplexityBefore float64
    ComplexityAfter  float64
    ComplexityDelta  float64 // Negative = simpler
}
```

### Step 3: Config Extension (15 min)

Add to config.yaml:

```yaml
quality:
  enabled: true
  lint_command: "golangci-lint run --json"
  test_command: "go test -json ./..."
  complexity_command: "gocyclo -over 0 -avg ./..."
  timeout: "5s"
```

### Step 4: Schema Update (15 min)

```sql
ALTER TABLE quality_metrics ADD COLUMN complexity_before REAL;
ALTER TABLE quality_metrics ADD COLUMN complexity_after REAL;
ALTER TABLE quality_metrics ADD COLUMN complexity_delta REAL;
```

### Step 5: Collector Integration (30 min)

Update `internal/quality/collector.go`:

```go
func (c *Collector) Snapshot(cwd string) *Snapshot {
    snapshot := &Snapshot{CapturedAt: time.Now()}
    
    // ... existing lint/test collection ...
    
    // Complexity
    if c.cfg.ComplexityCommand != "" {
        stats := MeasureComplexity(cwd, c.cfg.ComplexityCommand)
        if stats.Error == "" {
            snapshot.ComplexityAvg = stats.Average
            snapshot.ComplexityMax = stats.Max
        }
    }
    
    return snapshot
}
```

### Step 6: Tests (30 min)

Create `internal/quality/complexity_test.go`:

```go
func TestParseGocycloOutput(t *testing.T) {
    output := `5 main main.go:10:1
12 handleRequest server.go:45:1
3 helper util.go:5:1
Average: 6.67`
    
    stats := parseGocycloOutput(output)
    
    assert.Equal(t, 6.67, stats.Average)
    assert.Equal(t, 12, stats.Max)
    assert.Equal(t, 20, stats.Total)
    assert.Equal(t, 3, stats.FileCount)
}

func TestMeasureComplexityNotInstalled(t *testing.T) {
    stats := MeasureComplexity("/tmp", "nonexistent-tool")
    assert.NotEmpty(t, stats.Error)
}
```

---

## Todo List

- [ ] Create `internal/quality/complexity.go`
- [ ] Create `internal/quality/complexity_test.go`
- [ ] Update types with complexity fields
- [ ] Add config for complexity_command
- [ ] Add migration for complexity columns
- [ ] Integrate into collector
- [ ] Update status display
- [ ] Document gocyclo installation

---

## Success Criteria

1. `dandori status --run=X` shows complexity_delta when gocyclo available
2. Graceful fallback when tool not installed
3. Complexity measured in <2s (scoped to project)

---

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| gocyclo not installed | High | Low | Optional; warn once |
| Non-Go projects | High | Low | Config allows custom tool |
| Large codebase slow | Medium | Medium | Scope to changed files |

---

## Alternative Tools by Language

| Language | Tool | Command |
|----------|------|---------|
| Go | gocyclo | `gocyclo -over 0 -avg ./...` |
| JavaScript | complexity-report | `cr --format json src/` |
| Python | radon | `radon cc -a -s .` |
| Java | PMD | `pmd -R category/java/design.xml` |
| C# | dotnet-sonarscanner | via SonarQube |

---

## Next Steps

After Phase 03:
- Phase 04: Quality analytics and comparison views
