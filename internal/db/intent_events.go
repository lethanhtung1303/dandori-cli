package db

import (
	"encoding/json"
	"fmt"
)

// IntentData holds the G8 intent.extracted event payload for a run.
type IntentData struct {
	FirstUserMsg string        `json:"first_user_msg"`
	Summary      string        `json:"summary"`
	SpecLinks    IntentSpecLinks `json:"spec_links"`
}

// IntentSpecLinks mirrors the spec_links sub-object stored in the event payload.
type IntentSpecLinks struct {
	JiraKey        string   `json:"jira_key"`
	ConfluenceURLs []string `json:"confluence_urls"`
	SourcePaths    []string `json:"source_paths"`
}

// IntentDecision holds one decision.point event payload for a run.
type IntentDecision struct {
	Chosen    string   `json:"chosen"`
	Rejected  []string `json:"rejected,omitempty"`
	Rationale string   `json:"rationale,omitempty"`
}

// RunIntentEvents aggregates the G8 Layer-4 intent events for a single run.
// Intent is nil when no intent.extracted event exists for the run.
type RunIntentEvents struct {
	Intent    *IntentData
	Decisions []IntentDecision
}

// GetIntentEvents queries the events table for layer-4 G8 events belonging to
// runID and returns the parsed results. Returns nil Intent when the run has no
// intent.extracted event (e.g. legacy run or extraction was disabled).
//
// This function is fail-soft: any single event's JSON parse error is silently
// skipped so a malformed event does not block the Jira comment sync.
func (l *LocalDB) GetIntentEvents(runID string) (*RunIntentEvents, error) {
	rows, err := l.db.Query(`
		SELECT event_type, data
		FROM events
		WHERE run_id = ?
		  AND layer = 4
		  AND event_type IN ('intent.extracted', 'decision.point')
		ORDER BY id ASC
	`, runID)
	if err != nil {
		return nil, fmt.Errorf("query intent events: %w", err)
	}
	defer rows.Close()

	result := &RunIntentEvents{}

	for rows.Next() {
		var eventType, data string
		if err := rows.Scan(&eventType, &data); err != nil {
			// Skip malformed row — fail-soft.
			continue
		}

		switch eventType {
		case "intent.extracted":
			var d IntentData
			if err := json.Unmarshal([]byte(data), &d); err != nil {
				// Malformed payload — skip gracefully.
				continue
			}
			result.Intent = &d

		case "decision.point":
			var d IntentDecision
			if err := json.Unmarshal([]byte(data), &d); err != nil {
				continue
			}
			if d.Chosen != "" {
				result.Decisions = append(result.Decisions, d)
			}
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate intent events: %w", err)
	}

	return result, nil
}
