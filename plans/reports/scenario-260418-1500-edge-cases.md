# Scenario Report: dandori-cli Phase 01-05

## Packages Analyzed
config, db, model, event, wrapper, jira, server, sync

## Dimensions Analyzed
2 (Input Extremes), 3 (Timing), 5 (State Transitions), 7 (Error Cascades), 9 (Data Integrity), 10 (Integration)

## Dimensions Skipped
- 1 (User Types): CLI single-user
- 6 (Environment): Go cross-platform handled
- 8 (Authorization): API key only, simple
- 11 (Compliance): Not applicable yet
- 12 (Business Logic): No billing/pricing logic

---

## config package

| # | Dimension | Scenario | Severity | Expected Behavior |
|---|-----------|----------|----------|-------------------|
| 1 | Input Extremes | Empty config file | Medium | Use defaults |
| 2 | Input Extremes | Malformed YAML syntax | High | Return parse error |
| 3 | Input Extremes | YAML with unknown fields | Low | Ignore unknown fields |
| 4 | Input Extremes | Config path with unicode chars | Medium | Handle correctly |
| 5 | State Transitions | Config dir doesn't exist | Medium | Create on save |
| 6 | Error Cascades | HOME env not set | High | Return clear error |
| 7 | Error Cascades | Config file permission denied | High | Return permission error |
| 8 | Data Integrity | Env override with empty string | Medium | Keep YAML value |

---

## db package

| # | Dimension | Scenario | Severity | Expected Behavior |
|---|-----------|----------|----------|-------------------|
| 9 | Input Extremes | DB path with spaces | Medium | Handle with quoting |
| 10 | Input Extremes | Very long DB path (>4096) | Low | OS error |
| 11 | State Transitions | DB file doesn't exist | Medium | Create new |
| 12 | State Transitions | DB file corrupted | Critical | Return corruption error |
| 13 | State Transitions | Schema already migrated | Low | Skip migration |
| 14 | State Transitions | Partial migration failure | Critical | Rollback or clear error |
| 15 | Timing | Concurrent DB writes | High | WAL mode handles |
| 16 | Timing | DB locked by another process | High | Return busy error |
| 17 | Error Cascades | Disk full during write | Critical | Transaction rollback |

---

## wrapper package

| # | Dimension | Scenario | Severity | Expected Behavior |
|---|-----------|----------|----------|-------------------|
| 18 | Input Extremes | Empty command array | High | Return error |
| 19 | Input Extremes | Command with special chars | Medium | Pass through correctly |
| 20 | Input Extremes | Very long command (>100KB) | Low | OS limits |
| 21 | Timing | SIGINT during execution | High | Forward to child, cleanup |
| 22 | Timing | SIGTERM during execution | High | Forward to child, cleanup |
| 23 | Timing | Child process hangs forever | High | Respect context timeout |
| 24 | Timing | Rapid start/stop cycles | Medium | No resource leak |
| 25 | State Transitions | Git repo not initialized | Medium | Empty git HEAD |
| 26 | State Transitions | Session log dir doesn't exist | Medium | Skip tailer gracefully |
| 27 | State Transitions | Session log written mid-run | Medium | Tailer picks up |
| 28 | Error Cascades | DB insert fails | High | Still run command |
| 29 | Error Cascades | os.User() fails | Medium | Use fallback |
| 30 | Data Integrity | Run ID collision | Critical | UUID should be unique |
| 31 | Integration | Claude Code log format change | High | Parser returns empty |

---

## jira package

| # | Dimension | Scenario | Severity | Expected Behavior |
|---|-----------|----------|----------|-------------------|
| 32 | Input Extremes | Empty base URL | High | Return config error |
| 33 | Input Extremes | Invalid URL format | High | Return URL parse error |
| 34 | Input Extremes | Issue key with lowercase | Medium | Normalize to uppercase |
| 35 | Timing | API rate limit (429) | High | Retry with backoff |
| 36 | Timing | API timeout | High | Retry then fail |
| 37 | Timing | Concurrent API calls | Medium | Respect rate limits |
| 38 | Error Cascades | Jira server down (503) | High | Retry then fail |
| 39 | Error Cascades | Invalid credentials (401) | Critical | Clear auth error |
| 40 | Error Cascades | Issue not found (404) | Medium | Return not found |
| 41 | Integration | API response schema change | High | Graceful degradation |
| 42 | Integration | Webhook replay (duplicate) | Medium | Idempotent handling |
| 43 | Data Integrity | Sprint with 0 issues | Low | Return empty list |
| 44 | Data Integrity | Issue with null fields | Medium | Handle null gracefully |

---

## server package

| # | Dimension | Scenario | Severity | Expected Behavior |
|---|-----------|----------|----------|-------------------|
| 45 | Input Extremes | Empty JSON body | Medium | Return 400 |
| 46 | Input Extremes | Malformed JSON | High | Return 400 |
| 47 | Input Extremes | Very large batch (10K runs) | High | Process or reject |
| 48 | Timing | SSE client disconnect | Medium | Cleanup connection |
| 49 | Timing | Many SSE clients (1000+) | High | Connection limit |
| 50 | Timing | Slow DB during request | Medium | Request timeout |
| 51 | Error Cascades | PostgreSQL down | Critical | Health check fails |
| 52 | Error Cascades | DB connection pool exhausted | High | Queue or reject |
| 53 | Data Integrity | Duplicate run ID upsert | Medium | Update existing |
| 54 | Data Integrity | Run without workstation | High | FK constraint or null |
| 55 | Data Integrity | Event for non-existent run | High | FK constraint error |

---

## sync package

| # | Dimension | Scenario | Severity | Expected Behavior |
|---|-----------|----------|----------|-------------------|
| 56 | Input Extremes | Empty batch (0 runs) | Low | Return immediately |
| 57 | Input Extremes | Batch size 0 config | Medium | Use default |
| 58 | Timing | Server timeout during upload | High | Retry, don't mark synced |
| 59 | Timing | Partial success (some accepted) | High | Mark only accepted |
| 60 | Error Cascades | Server unreachable | High | Return network error |
| 61 | Error Cascades | Server returns 500 | High | Don't mark synced |
| 62 | Error Cascades | Local DB read fails | High | Return DB error |
| 63 | Data Integrity | Already synced re-upload | Low | Server upserts |
| 64 | Data Integrity | Sync interrupted mid-batch | Critical | Idempotent on retry |

---

## Summary

| Severity | Count |
|----------|-------|
| Critical | 8 |
| High | 29 |
| Medium | 21 |
| Low | 6 |
| **Total** | **64 scenarios** |

## Test Priority

1. **Critical (8)**: DB corruption, disk full, run ID collision, sync interruption, auth failure, PostgreSQL down, partial migration, FK violations
2. **High (29)**: All error cascades, timing issues, input validation
3. **Medium/Low (27)**: Edge cases, graceful degradation
