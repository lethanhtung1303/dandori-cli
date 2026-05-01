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

// TotalRunIDs returns IDs of all runs whose started_at falls in [since, until).
// Filters by department when team is non-empty. ALL statuses included
// (done|error|cancelled|running) — cancelled stays in the denominator on
// purpose so teams can't game Rework Rate by cancelling iteration runs.
func (l *LocalDB) TotalRunIDs(since, until time.Time, team string) ([]string, error) {
	q := `SELECT id FROM runs WHERE started_at >= ? AND started_at < ?`
	args := []any{since.UTC().Format(time.RFC3339), until.UTC().Format(time.RFC3339)}
	if team != "" {
		q += ` AND COALESCE(department, '') = ?`
		args = append(args, team)
	}
	rows, err := l.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("query total runs: %w", err)
	}
	defer rows.Close()
	return collectStrings(rows)
}

// ReworkRunIDs returns distinct run IDs that have at least one
// task.iteration.start event with round >= 2 AND whose run started in the
// window. The run-level start filter avoids counting iteration events from
// older runs whose ticket was reopened during the window.
func (l *LocalDB) ReworkRunIDs(since, until time.Time, team string) ([]string, error) {
	q := `
		SELECT DISTINCT r.id
		FROM runs r
		JOIN events e ON e.run_id = r.id
		WHERE e.event_type = 'task.iteration.start'
		  AND CAST(json_extract(e.data, '$.round') AS INTEGER) >= 2
		  AND r.started_at >= ? AND r.started_at < ?`
	args := []any{since.UTC().Format(time.RFC3339), until.UTC().Format(time.RFC3339)}
	if team != "" {
		q += ` AND COALESCE(r.department, '') = ?`
		args = append(args, team)
	}
	rows, err := l.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("query rework runs: %w", err)
	}
	defer rows.Close()
	return collectStrings(rows)
}

func collectStrings(rows *sql.Rows) ([]string, error) {
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// IterationBucket is one row of the iteration-count histogram returned by
// IterationDistribution: a bucket label ("1", "2", "3", "4", "5+") and the
// number of distinct tasks that fall in it.
type IterationBucket struct {
	Label string
	Count int
}

// IterationDistribution returns a histogram of tasks bucketed by their
// iteration round count. Round count = 1 + (count of task.iteration.start
// events for that issue). The +1 accounts for the implicit first round
// (no event emitted for round 1, mirroring IterationStats semantics).
//
// Window scopes to runs whose started_at falls in [since, until). When
// projectKey != "", scopes to runs whose jira_issue_key starts with
// "<projectKey>-". Tasks with empty jira_issue_key are excluded.
//
// Returns 5 buckets in canonical order: "1", "2", "3", "4", "5+".
func (l *LocalDB) IterationDistribution(since, until time.Time, projectKey string) ([]IterationBucket, int, error) {
	q := `
		WITH per_task AS (
			SELECT r.jira_issue_key,
			       1 + (
			           SELECT COUNT(*) FROM events e
			           JOIN runs r2 ON e.run_id = r2.id
			           WHERE r2.jira_issue_key = r.jira_issue_key
			             AND e.event_type = 'task.iteration.start'
			       ) AS round_count
			FROM runs r
			WHERE r.jira_issue_key IS NOT NULL AND r.jira_issue_key != ''
			  AND r.started_at >= ? AND r.started_at < ?`
	args := []any{since.UTC().Format(time.RFC3339), until.UTC().Format(time.RFC3339)}
	if projectKey != "" {
		q += ` AND r.jira_issue_key LIKE ?`
		args = append(args, projectKey+"-%")
	}
	q += `
			GROUP BY r.jira_issue_key
		)
		SELECT round_count FROM per_task
	`
	rows, err := l.db.Query(q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("iteration distribution query: %w", err)
	}
	defer rows.Close()

	counts := map[string]int{"1": 0, "2": 0, "3": 0, "4": 0, "5+": 0}
	total := 0
	for rows.Next() {
		var r int
		if err := rows.Scan(&r); err != nil {
			return nil, 0, err
		}
		switch {
		case r <= 1:
			counts["1"]++
		case r == 2:
			counts["2"]++
		case r == 3:
			counts["3"]++
		case r == 4:
			counts["4"]++
		default:
			counts["5+"]++
		}
		total++
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	order := []string{"1", "2", "3", "4", "5+"}
	out := make([]IterationBucket, len(order))
	for i, label := range order {
		out[i] = IterationBucket{Label: label, Count: counts[label]}
	}
	return out, total, nil
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
