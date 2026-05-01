package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/phuc-nt/dandori-cli/internal/db"
)

type engineerDetailResp struct {
	Engineer           string             `json:"engineer"`
	Runs               []engineerDetailRun `json:"runs"`
	RetentionSparkline []float64          `json:"retention_sparkline"`
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

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}
