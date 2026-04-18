# E2E Comprehensive Test Plan

> Comprehensive end-to-end test cases for dandori-cli with real Jira, Confluence, and Claude Code.

## Evaluation of Existing E2E Tests

**Current coverage** (`internal/integration/e2e_test.go`):
- ✅ Sprint fetch + agent suggest + comment
- ✅ Confluence context assembly
- ✅ Jira poller single poll
- ❌ No real Claude Code execution
- ❌ No token/cost accuracy verification
- ❌ No CLI command coverage (task, jira-sync, conf-write, analytics, dashboard)
- ❌ No DB state verification
- ❌ No failure scenarios
- ❌ No multi-run scenarios

## Test Case Groups

### Group A: Configuration & Setup (3 cases)

| ID | Test Case | Verify |
|----|-----------|--------|
| A1 | `dandori init` creates config | Config file + DB exists |
| A2 | `dandori version` shows version | Output non-empty |
| A3 | Config loads Jira/Confluence credentials | Connection tests pass |

### Group B: Jira Task Lifecycle (4 cases)

| ID | Test Case | Verify |
|----|-----------|--------|
| B1 | `task info` on existing task | Key, summary, status returned |
| B2 | `task start` on To Do task | Status → In Progress |
| B3 | `task start` adds starting comment | Jira shows comment with agent name |
| B4 | `task done` manual transition | Status → Done |

### Group C: Agent Execution (Real Claude) (5 cases)

| ID | Test Case | Verify |
|----|-----------|--------|
| C1 | Read-only task (no file write) | Run tracked, exit 0 |
| C2 | Simple file creation | File exists, run tracked |
| C3 | Multi-step task (read + write) | Multiple tool uses logged |
| C4 | Task with explicit failure | Exit code non-zero, status=failed |
| C5 | Quick task (<3s) | Session detected, tokens captured |

### Group D: Tracking Accuracy (7 cases)

| ID | Test Case | Verify |
|----|-----------|--------|
| D1 | Run ID stored | DB has row with run ID |
| D2 | Exit code captured | DB exit_code matches actual |
| D3 | Duration captured | DB duration_sec > 0 |
| D4 | Git HEAD before captured | DB git_head_before non-empty |
| D5 | Input/output tokens > 0 | Real Claude session parsed |
| D6 | Cost > 0 | Cost = tokens * model price |
| D7 | Model name captured | DB model = "claude-sonnet-4-6" |

### Group E: Jira Sync (5 cases)

| ID | Test Case | Verify |
|----|-----------|--------|
| E1 | `jira-sync --dry-run` preview | Shows would-sync runs, no API calls |
| E2 | `jira-sync` transitions Done | Jira status = Done |
| E3 | `jira-sync` adds completion comment | Comment with cost/duration |
| E4 | Re-run jira-sync no duplicates | Synced runs skipped |
| E5 | `jira-sync --task X` filters | Only target task synced |

### Group F: Confluence Reporting (6 cases)

| ID | Test Case | Verify |
|----|-----------|--------|
| F1 | `conf-write --dry-run` preview | Title + body output |
| F2 | `conf-write --task X` creates page | Page ID returned |
| F3 | Report contains token data | Input/output tokens in body |
| F4 | Report contains cost | Cost USD in body |
| F5 | Report has git commits | Git HEAD before/after in body |
| F6 | Multiple tasks → multiple pages | Each task has unique page |

### Group G: Analytics (5 cases)

| ID | Test Case | Verify |
|----|-----------|--------|
| G1 | `analytics runs` lists runs | All test runs shown |
| G2 | `analytics agents` shows stats | Agent name, run count, cost |
| G3 | `analytics cost` aggregates | Total = sum of individual |
| G4 | Success rate calculated | % = successful / total |
| G5 | Token total aggregates | Total tokens match |

### Group H: Dashboard (4 cases)

| ID | Test Case | Verify |
|----|-----------|--------|
| H1 | Dashboard server starts | Port 8088 listening |
| H2 | `/api/overview` returns totals | Runs, cost, tokens JSON |
| H3 | `/api/runs` returns list | Array of run objects |
| H4 | Dashboard HTML accessible | 200 OK on GET / |

### Group I: Edge Cases (3 cases)

| ID | Test Case | Verify |
|----|-----------|--------|
| I1 | `task info` on invalid key | Error returned, no crash |
| I2 | `conf-write --task NONEXISTENT` | Graceful error |
| I3 | Run with invalid command | Failure tracked |

### Group J: Long-running / Heavy Task (5 cases)

| ID | Test Case | Verify |
|----|-----------|--------|
| J1 | Multi-step task (read + analyze + write) | Completes without error |
| J2 | Heavy task has > 100 tokens | Token count reflects work |
| J3 | Heavy task has non-zero cost | Cost > 0 |
| J4 | Duration >= 3 seconds | Real long-running capture |
| J5 | Heavy run syncs to Jira Done | Full pipeline on long task |

### Group K: Shell Alias Transparency (5 cases)

| ID | Test Case | Verify |
|----|-----------|--------|
| K1 | `dandori init --shell` adds alias block | RC file contains marker + alias |
| K2 | Re-running init is idempotent | Alias block not duplicated |
| K3 | `dandori init --no-shell` skips aliases | RC file unchanged |
| K4 | Auto-detects shell from $SHELL | zsh→.zshrc, bash→.bashrc |
| K5 | Alias block has uninstall marker | Has both start and end markers |

### Group L: Watch Daemon (5 cases)

| ID | Test Case | Verify |
|----|-----------|--------|
| L1 | `dandori watch --once` exits after one poll | Returns 0 |
| L2 | Watch lists existing session dirs | Finds ~/.claude/projects/ |
| L3 | Watch creates orphan run for new session | DB row added with jira_issue_key NULL |
| L4 | Watch extracts tokens from session | input_tokens > 0 captured |
| L5 | Re-running watch skips already-tracked sessions | No duplicate rows |

## Summary

**Total: 57 test cases across 12 groups**

- Configuration: 3
- Jira Lifecycle: 4
- Agent Execution: 5
- Tracking: 7
- Jira Sync: 5
- Confluence: 6
- Analytics: 5
- Dashboard: 4
- Edge Cases: 3
- Heavy Task: 5

## Execution Strategy

1. **Clear test data** — Delete local DB, note existing Jira tasks
2. **Create fresh tasks** — 3 new Jira tasks for different scenarios
3. **Run test script** — `scripts/e2e-comprehensive.sh`
4. **Generate report** — `plans/reports/e2e-report-260418-2051.md`
5. **Verify pass/fail** — Each case has assertion

## Test Data Requirements

- Jira project: CLITEST (already configured)
- Confluence space: CLITEST (already configured)
- Claude Code: installed and working
- At least 3 new tasks to create
