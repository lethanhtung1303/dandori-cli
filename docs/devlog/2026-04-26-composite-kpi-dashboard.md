# 2026-04-26 — Composite KPI queries (Phase 02) — Web Dashboard

## Summary

Surfaced the 3 quality KPIs (regression rate, bug rate, quality-adjusted cost) on the
`dandori dashboard` (port 8088, local SQLite). Each KPI has its own card with a dimension
dropdown (By Agent / By Engineer / By Sprint). Dropdowns independently re-fetch only their
own table. Auto-refresh (30s) respects each card's current dropdown value.

## What shipped

### New files
- `cmd/dashboard_test.go` — 11 handler tests (3 endpoints × 3 dims + invalid `by` 400 + empty DB)

### Modified files
- `cmd/dashboard.go` (+272 LOC):
  - Extracted `newDashboardMux(store, jiraBaseURL)` helper (testability refactor)
  - Added `qualityHandler(store, kpi)` factory — validates `by`, parses `since`/`top`, calls Phase 01 query funcs
  - Added `atoiOr(s, def)` helper
  - Registered `/api/quality/regression`, `/api/quality/bugs`, `/api/quality/cost`
  - Added Quality KPI nav item in sidebar
  - Added `.dim-selector` CSS + `.clean-badge` CSS
  - Added 3 stacked Quality KPI cards (regression/bugs/cost) with `<select class="dim-selector">` per card
  - Added `loadQualityRegression()`, `loadQualityBugs()`, `loadQualityCost()` JS functions
  - Extended `loadAll()` to call all 3 quality loaders
  - Added `.dim-selector` change event wiring

## API surface

| Endpoint | Params | Returns |
|---|---|---|
| `GET /api/quality/regression` | `?by=agent\|engineer\|sprint&since=N` | `[]RegressionRow` |
| `GET /api/quality/bugs` | `?by=agent\|engineer\|sprint&since=N` | `[]BugRateRow` |
| `GET /api/quality/cost` | `?by=agent\|engineer\|sprint&since=N&top=N` | `[]TaskCostRow` |

All three reuse Phase 01 `internal/db` query funcs verbatim — no SQL duplication.

## Test results

- 11/11 handler tests pass
- Full `go test ./...` clean — no regressions

## Scope notes

- SQLite-only (port 8088) — Postgres fleet dashboard (port 8080) out of scope per plan Q5
- `dashboard.go` reached 1463 LOC (was 1191); single-file convention preserved per Phuc decision
