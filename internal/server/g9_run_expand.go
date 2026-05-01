package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/phuc-nt/dandori-cli/internal/db"
)

type runExpandResp struct {
	RunID        string                 `json:"run_id"`
	IssueKey     string                 `json:"issue_key"`
	Iterations   []runExpandIteration   `json:"iterations"`
	IntentEvents []runExpandIntentEvent `json:"intent_events"`
}

type runExpandIteration struct {
	Round          int    `json:"round"`
	IssueKey       string `json:"issue_key"`
	TransitionedAt string `json:"transitioned_at"`
}

type runExpandIntentEvent struct {
	EventType string          `json:"event_type"`
	TS        string          `json:"ts"`
	Data      json.RawMessage `json:"data"`
}

// handleG9RunExpand serves /api/g9/run/{runID}/expand.
// Returns iterations (task.iteration.start events scoped to the run's issue
// key) plus layer-4 intent events for the run. Used by Recent Runs row click.
func handleG9RunExpand(store *db.LocalDB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/g9/run/"), "/expand")
		if runID == "" || strings.Contains(runID, "/") {
			http.Error(w, `{"error":"missing run id"}`, http.StatusBadRequest)
			return
		}

		// Lookup the run to get its jira_issue_key. 404 if not found.
		var issueKey string
		err := store.QueryRow(`SELECT COALESCE(jira_issue_key, '') FROM runs WHERE id = ?`, runID).Scan(&issueKey)
		if err != nil {
			http.Error(w, `{"error":"run not found"}`, http.StatusNotFound)
			return
		}

		resp := runExpandResp{
			RunID:        runID,
			IssueKey:     issueKey,
			Iterations:   []runExpandIteration{},
			IntentEvents: []runExpandIntentEvent{},
		}

		// Iterations: scoped by issue key (events table joins back via run row).
		if issueKey != "" {
			iterRows, err := store.IterationEventsForIssue(issueKey)
			if err == nil {
				for _, it := range iterRows {
					resp.Iterations = append(resp.Iterations, runExpandIteration{
						Round:          it.Round,
						IssueKey:       it.IssueKey,
						TransitionedAt: it.TransitionedAt.Format(time.RFC3339),
					})
				}
			}
		}

		// Intent events: layer 4 events for this run.
		eventRows, err := store.Query(`
			SELECT event_type, ts, data
			FROM events
			WHERE run_id = ? AND layer = 4
			ORDER BY ts ASC
		`, runID)
		if err == nil {
			defer eventRows.Close()
			for eventRows.Next() {
				var et, ts, data string
				if err := eventRows.Scan(&et, &ts, &data); err != nil {
					continue
				}
				resp.IntentEvents = append(resp.IntentEvents, runExpandIntentEvent{
					EventType: et,
					TS:        ts,
					Data:      json.RawMessage(data),
				})
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}
