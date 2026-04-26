package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// LatestRun is the slice of run state the iteration detector needs. Defined
// here to avoid pulling jira→db→model into a cycle; jira package wraps it.
type LatestRun struct {
	ID                       string
	Status                   string
	JiraIssueKey             string
	JiraStatusAtCompletion   string
	JiraCategoryAtCompletion string
	StartedAt                time.Time
	EndedAt                  time.Time
}

// IterationEventRow is a parsed task.iteration.start event row from events.
type IterationEventRow struct {
	Round          int
	IssueKey       string
	TransitionedAt time.Time
}

// LatestRunForIssue returns the most recently started run for the given Jira
// issue, or nil if none exists. Used by the poller to decide whether the
// current Jira status represents a regression from a prior completion.
//
// jira_status_at_completion / jira_category_at_completion aren't yet stored on
// the runs table — left blank for now. Phase 03 follow-up will record them
// when wrapper finishes a run; until then the detector treats blank category
// as "not done", so iteration detection silently no-ops. This is the safest
// default: false negatives, never false positives.
func (l *LocalDB) LatestRunForIssue(issueKey string) (*LatestRun, error) {
	row := l.db.QueryRow(`
		SELECT id, COALESCE(status, ''), COALESCE(jira_issue_key, ''),
		       COALESCE(started_at, ''), COALESCE(ended_at, '')
		FROM runs
		WHERE jira_issue_key = ?
		ORDER BY started_at DESC
		LIMIT 1
	`, issueKey)

	var r LatestRun
	var startedAt, endedAt string
	if err := row.Scan(&r.ID, &r.Status, &r.JiraIssueKey, &startedAt, &endedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("scan latest run: %w", err)
	}
	r.StartedAt, _ = time.Parse(time.RFC3339, startedAt)
	r.EndedAt, _ = time.Parse(time.RFC3339, endedAt)

	// Map dandori run status → Jira-like category for the detector. A run that
	// finished cleanly (status=done) means the agent completed the work, which
	// implies the Jira issue was in a done category at completion time.
	if r.Status == "done" {
		r.JiraCategoryAtCompletion = "done"
		r.JiraStatusAtCompletion = "Done"
	}
	return &r, nil
}

// IterationEventsForIssue returns all task.iteration.start rows for the issue,
// parsed from events.data JSON. Events are linked via runs.jira_issue_key
// (events table itself doesn't carry issue_key — it's denormalised through
// the run row).
func (l *LocalDB) IterationEventsForIssue(issueKey string) ([]IterationEventRow, error) {
	rows, err := l.db.Query(`
		SELECT e.data
		FROM events e
		JOIN runs r ON e.run_id = r.id
		WHERE r.jira_issue_key = ? AND e.event_type = 'task.iteration.start'
	`, issueKey)
	if err != nil {
		return nil, fmt.Errorf("query iteration events: %w", err)
	}
	defer rows.Close()

	var out []IterationEventRow
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		var payload struct {
			Round          int    `json:"round"`
			IssueKey       string `json:"issue_key"`
			TransitionedAt string `json:"transitioned_at"`
		}
		if err := json.Unmarshal([]byte(data), &payload); err != nil {
			continue
		}
		ts, _ := time.Parse(time.RFC3339, payload.TransitionedAt)
		out = append(out, IterationEventRow{
			Round:          payload.Round,
			IssueKey:       payload.IssueKey,
			TransitionedAt: ts,
		})
	}
	return out, nil
}
