// Package server — g9_rework.go: /api/g9/rework handler.
// Surfaces Rework Rate (rework_runs / total_runs in window) as a hero tile
// on org + project views. Reuses the same definition as
// `dandori metric export` but applies a Jira project-key filter when scoped.
package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/phuc-nt/dandori-cli/internal/db"
)

// reworkAlertThreshold matches metric.ReworkThresholdV1; surfaced as a
// constant here so the handler can flag tiles without importing the whole
// metric package. If the underlying threshold ever changes, update both.
const reworkAlertThreshold = 0.10

// handleG9Rework handles GET /api/g9/rework.
// Query: ?scope=org|project, ?id=<jira-key-prefix>, ?period=<days> (default 28).
// Response: {rate, total, rework, exceeds_threshold, period_days,
//
//	wow_delta_pp, prior_rate, empty}.
func handleG9Rework(store *db.LocalDB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		q := r.URL.Query()
		scope := q.Get("scope")
		id := q.Get("id")
		days := 28
		if v := q.Get("period"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				days = n
			}
		}

		now := time.Now()
		curStart := now.AddDate(0, 0, -days)
		priorStart := now.AddDate(0, 0, -2*days)

		projectKey := ""
		if scope == "project" && id != "" {
			projectKey = id
		}

		curTotal, curRework, err := reworkCounts(store, curStart, now, projectKey)
		if err != nil {
			http.Error(w, `{"error":"rework query failed"}`, http.StatusInternalServerError)
			return
		}

		resp := map[string]any{
			"period_days": days,
			"total":       curTotal,
			"rework":      curRework,
		}
		if curTotal == 0 {
			resp["empty"] = true
			resp["rate"] = nil
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		curRate := float64(curRework) / float64(curTotal)
		resp["rate"] = curRate
		resp["exceeds_threshold"] = curRate > reworkAlertThreshold

		// Prior window — used for WoW delta in percentage points.
		priorTotal, priorRework, err := reworkCounts(store, priorStart, curStart, projectKey)
		if err == nil && priorTotal > 0 {
			priorRate := float64(priorRework) / float64(priorTotal)
			resp["prior_rate"] = priorRate
			resp["wow_delta_pp"] = (curRate - priorRate) * 100
		}

		_ = json.NewEncoder(w).Encode(resp)
	}
}

// reworkCounts returns (total, rework) run counts in [start, end) optionally
// filtered to runs whose jira_issue_key starts with "<projectKey>-".
// "rework" runs are those whose run_id appears in events with type
// task.iteration.start AND round >= 2 — same definition as
// db.ReworkRunIDs but with the extra Jira-project filter inlined.
func reworkCounts(store *db.LocalDB, start, end time.Time, projectKey string) (total, rework int, err error) {
	startStr := start.UTC().Format(time.RFC3339)
	endStr := end.UTC().Format(time.RFC3339)

	totalQ := `SELECT COUNT(*) FROM runs WHERE started_at >= ? AND started_at < ?`
	totalArgs := []any{startStr, endStr}
	if projectKey != "" {
		totalQ += ` AND jira_issue_key LIKE ?`
		totalArgs = append(totalArgs, projectKey+"-%")
	}
	if err := store.QueryRow(totalQ, totalArgs...).Scan(&total); err != nil {
		return 0, 0, err
	}

	reworkQ := `
		SELECT COUNT(DISTINCT r.id)
		FROM runs r
		JOIN events e ON e.run_id = r.id
		WHERE e.event_type = 'task.iteration.start'
		  AND CAST(json_extract(e.data, '$.round') AS INTEGER) >= 2
		  AND r.started_at >= ? AND r.started_at < ?`
	reworkArgs := []any{startStr, endStr}
	if projectKey != "" {
		reworkQ += ` AND r.jira_issue_key LIKE ?`
		reworkArgs = append(reworkArgs, projectKey+"-%")
	}
	if err := store.QueryRow(reworkQ, reworkArgs...).Scan(&rework); err != nil {
		return 0, 0, err
	}

	return total, rework, nil
}
