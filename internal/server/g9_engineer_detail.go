package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/phuc-nt/dandori-cli/internal/db"
)

type engineerDetailResp struct {
	Engineer           string              `json:"engineer"`
	Runs               []engineerDetailRun `json:"runs"`
	RetentionSparkline []float64           `json:"retention_sparkline"`
	KPI7d              engineerKPI         `json:"kpi_7d"`
	WoW                engineerWoW         `json:"wow"`
	Empty              bool                `json:"empty"`
}

// engineerKPI is the 7-day hero-strip aggregate. Empty engineer (no runs)
// emits zeros + Empty=true so the frontend can render `—` placeholders
// instead of misleading 0% values.
type engineerKPI struct {
	Cost7d          float64 `json:"cost_7d"`
	Runs7d          int     `json:"runs_7d"`
	Interventions7d int     `json:"interventions_7d"`
	AutonomyPct     float64 `json:"autonomy_pct"`
	SuccessPct      float64 `json:"success_pct"`
}

// engineerWoW captures cost-WoW for the hero strip. Prior window is the
// 7-day block immediately preceding the current 7-day window.
type engineerWoW struct {
	CostPriorUSD float64 `json:"cost_prior_usd"`
	CostDeltaPct float64 `json:"cost_delta_pct"`
}

type engineerDetailRun struct {
	ID           string  `json:"id"`
	JiraIssueKey string  `json:"jira_issue_key"`
	AgentName    string  `json:"agent_name"`
	EngineerName string  `json:"engineer_name"`
	StartedAt    string  `json:"started_at"`
	CostUSD      float64 `json:"cost_usd"`
	Status       string  `json:"status"`
}

// handleG9EngineerDetail serves /api/g9/engineer/{name}.
// Returns the latest 50 runs for the engineer + a 4-bucket weekly retention
// sparkline over the last 28 days. Tolerant of unknown names: empty arrays,
// 4 zero-buckets, status 200.
func handleG9EngineerDetail(store *db.LocalDB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/api/g9/engineer/")
		if name == "" || strings.Contains(name, "/") {
			http.Error(w, `{"error":"missing engineer name"}`, http.StatusBadRequest)
			return
		}

		resp := engineerDetailResp{
			Engineer:           name,
			Runs:               []engineerDetailRun{},
			RetentionSparkline: []float64{0, 0, 0, 0},
		}

		// Latest 50 runs.
		rows, err := store.Query(`
			SELECT id, COALESCE(jira_issue_key,''), COALESCE(agent_name,''),
			       COALESCE(engineer_name,''), started_at, COALESCE(cost_usd, 0),
			       COALESCE(status,'')
			FROM runs
			WHERE engineer_name = ?
			ORDER BY started_at DESC
			LIMIT 50
		`, name)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var run engineerDetailRun
				if err := rows.Scan(&run.ID, &run.JiraIssueKey, &run.AgentName,
					&run.EngineerName, &run.StartedAt, &run.CostUSD, &run.Status); err == nil {
					resp.Runs = append(resp.Runs, run)
				}
			}
		}

		// Retention sparkline: 4 weekly buckets over last 28 days.
		// Bucket value = success_rate (1 - intervention_rate proxy) per week.
		// Simpler proxy: runs_per_week / max_runs_per_week, normalized 0-1.
		now := time.Now()
		buckets := make([]float64, 4)
		for i := 0; i < 4; i++ {
			weekStart := now.Add(-time.Duration((i+1)*7) * 24 * time.Hour)
			weekEnd := now.Add(-time.Duration(i*7) * 24 * time.Hour)
			var runCount, interventionSum int
			err := store.QueryRow(`
				SELECT COUNT(*), COALESCE(SUM(human_intervention_count), 0)
				FROM runs
				WHERE engineer_name = ?
				  AND started_at >= ?
				  AND started_at < ?
			`, name, weekStart.UTC().Format(time.RFC3339), weekEnd.UTC().Format(time.RFC3339)).
				Scan(&runCount, &interventionSum)
			if err != nil || runCount == 0 {
				buckets[3-i] = 0 // oldest week at index 0, newest at 3
				continue
			}
			// Lower interventions/run = higher retention. Cap rate at 1.0.
			rate := float64(interventionSum) / float64(runCount)
			retention := 1.0 - rate
			if retention < 0 {
				retention = 0
			}
			if retention > 1 {
				retention = 1
			}
			buckets[3-i] = retention
		}
		resp.RetentionSparkline = buckets

		// 7-day KPI strip (current + prior windows).
		curStart := now.Add(-7 * 24 * time.Hour)
		priorStart := now.Add(-14 * 24 * time.Hour)
		resp.KPI7d = engineerKPIWindow(store, name, curStart, now)
		priorKPI := engineerKPIWindow(store, name, priorStart, curStart)

		resp.WoW.CostPriorUSD = priorKPI.Cost7d
		if priorKPI.Cost7d > 0 {
			resp.WoW.CostDeltaPct = (resp.KPI7d.Cost7d - priorKPI.Cost7d) / priorKPI.Cost7d * 100
		}

		// Empty=true when the engineer has zero runs in the current window.
		// Frontend renders `—` placeholders instead of "0%" which would mislead.
		if resp.KPI7d.Runs7d == 0 {
			resp.Empty = true
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

// engineerKPIWindow aggregates one engineer's runs over [start, end).
// Used for current and prior 7-day windows. Errors are swallowed: a query
// failure returns zero values so the strip still renders.
func engineerKPIWindow(store *db.LocalDB, name string, start, end time.Time) engineerKPI {
	var k engineerKPI
	row := store.QueryRow(`
		SELECT COUNT(*) AS runs,
		       COALESCE(SUM(cost_usd), 0) AS cost,
		       COALESCE(SUM(human_intervention_count), 0) AS intervs,
		       COALESCE(SUM(CASE WHEN COALESCE(human_intervention_count,0)=0 THEN 1 ELSE 0 END), 0) AS autonomous_runs,
		       COALESCE(SUM(CASE WHEN status='done' THEN 1 ELSE 0 END), 0) AS done_runs
		FROM runs
		WHERE engineer_name = ?
		  AND started_at >= ?
		  AND started_at < ?
	`, name,
		start.UTC().Format(time.RFC3339),
		end.UTC().Format(time.RFC3339))

	var autonomous, done int
	if err := row.Scan(&k.Runs7d, &k.Cost7d, &k.Interventions7d, &autonomous, &done); err != nil {
		return k
	}
	if k.Runs7d > 0 {
		k.AutonomyPct = float64(autonomous) / float64(k.Runs7d) * 100
		k.SuccessPct = float64(done) / float64(k.Runs7d) * 100
	}
	return k
}
