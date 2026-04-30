package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/phuc-nt/dandori-cli/internal/quality"
)

// RunRecord holds the run row data needed for an incident report.
type RunRecord struct {
	ID            string
	JiraIssueKey  string
	AgentName     string
	Model         string
	StartedAt     time.Time
	DurationSec   float64
	ExitCode      int
	Status        string
	CostUSD       float64
	InputTokens   int
	OutputTokens  int
	GitHeadBefore string
	GitHeadAfter  string
}

// ToolUsageSummary aggregates tool.use events by tool name for a single run.
type ToolUsageSummary struct {
	Tool  string
	Count int
}

// GetRunRecord returns the run metadata for a single run ID.
// Returns sql.ErrNoRows wrapped when not found.
func (l *LocalDB) GetRunRecord(runID string) (*RunRecord, error) {
	var r RunRecord
	var startedAt string
	var jiraKey, agentName, model, gitBefore, gitAfter sql.NullString
	var durationSec sql.NullFloat64
	var exitCode sql.NullInt32
	var inputTokens, outputTokens sql.NullInt32

	err := l.QueryRow(`
		SELECT id,
		       COALESCE(jira_issue_key, ''),
		       COALESCE(agent_name, ''),
		       COALESCE(model, ''),
		       started_at,
		       COALESCE(duration_sec, 0),
		       COALESCE(exit_code, -1),
		       status,
		       COALESCE(cost_usd, 0),
		       COALESCE(input_tokens, 0),
		       COALESCE(output_tokens, 0),
		       COALESCE(git_head_before, ''),
		       COALESCE(git_head_after, '')
		FROM runs WHERE id = ?
	`, runID).Scan(
		&r.ID, &jiraKey, &agentName, &model,
		&startedAt,
		&durationSec, &exitCode,
		&r.Status, &r.CostUSD,
		&inputTokens, &outputTokens,
		&gitBefore, &gitAfter,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("run %q not found", runID)
	}
	if err != nil {
		return nil, fmt.Errorf("get run record: %w", err)
	}

	r.JiraIssueKey = jiraKey.String
	r.AgentName = agentName.String
	r.Model = model.String
	r.DurationSec = durationSec.Float64
	r.ExitCode = int(exitCode.Int32)
	r.InputTokens = int(inputTokens.Int32)
	r.OutputTokens = int(outputTokens.Int32)
	r.GitHeadBefore = gitBefore.String
	r.GitHeadAfter = gitAfter.String
	r.StartedAt, _ = time.Parse(time.RFC3339, startedAt)

	return &r, nil
}

// GetRunsForJiraKey returns all runs for a Jira issue key ordered by started_at DESC.
// Optional since filter (zero value = no filter).
func (l *LocalDB) GetRunsForJiraKey(jiraKey string, since time.Time) ([]*RunRecord, error) {
	query := `
		SELECT id,
		       COALESCE(jira_issue_key, ''),
		       COALESCE(agent_name, ''),
		       COALESCE(model, ''),
		       started_at,
		       COALESCE(duration_sec, 0),
		       COALESCE(exit_code, -1),
		       status,
		       COALESCE(cost_usd, 0),
		       COALESCE(input_tokens, 0),
		       COALESCE(output_tokens, 0),
		       COALESCE(git_head_before, ''),
		       COALESCE(git_head_after, '')
		FROM runs
		WHERE jira_issue_key = ?
	`
	args := []any{jiraKey}

	if !since.IsZero() {
		query += " AND started_at >= ?"
		args = append(args, since.Format(time.RFC3339))
	}
	query += " ORDER BY started_at DESC"

	rows, err := l.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query runs for jira key: %w", err)
	}
	defer rows.Close()

	var result []*RunRecord
	for rows.Next() {
		var r RunRecord
		var startedAt string
		err := rows.Scan(
			&r.ID, &r.JiraIssueKey, &r.AgentName, &r.Model,
			&startedAt,
			&r.DurationSec, &r.ExitCode,
			&r.Status, &r.CostUSD,
			&r.InputTokens, &r.OutputTokens,
			&r.GitHeadBefore, &r.GitHeadAfter,
		)
		if err != nil {
			return nil, fmt.Errorf("scan run record: %w", err)
		}
		r.StartedAt, _ = time.Parse(time.RFC3339, startedAt)
		result = append(result, &r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate runs: %w", err)
	}
	return result, nil
}

// GetToolUsageForRun returns tool.use event counts grouped by tool name for a run.
// Returns top N rows ordered by count descending.
func (l *LocalDB) GetToolUsageForRun(runID string, top int) ([]ToolUsageSummary, error) {
	if top <= 0 {
		top = 5
	}
	rows, err := l.db.Query(`
		SELECT json_extract(data, '$.tool') AS tool,
		       COUNT(*) AS cnt
		FROM events
		WHERE run_id = ?
		  AND event_type = 'tool.use'
		  AND json_extract(data, '$.tool') IS NOT NULL
		GROUP BY tool
		ORDER BY cnt DESC
		LIMIT ?
	`, runID, top)
	if err != nil {
		return nil, fmt.Errorf("tool usage for run: %w", err)
	}
	defer rows.Close()

	var result []ToolUsageSummary
	for rows.Next() {
		var s ToolUsageSummary
		if err := rows.Scan(&s.Tool, &s.Count); err != nil {
			return nil, fmt.Errorf("scan tool usage: %w", err)
		}
		result = append(result, s)
	}
	return result, rows.Err()
}

// GetReasoningTrace returns top N layer-3 reasoning events for a run, truncated to maxChars.
func (l *LocalDB) GetReasoningTrace(runID string, top, maxChars int) ([]string, error) {
	if top <= 0 {
		top = 5
	}
	if maxChars <= 0 {
		maxChars = 300
	}
	rows, err := l.db.Query(`
		SELECT COALESCE(data, '')
		FROM events
		WHERE run_id = ?
		  AND layer = 3
		  AND event_type IN ('agent.thinking', 'agent.reasoning', 'reasoning')
		ORDER BY id ASC
		LIMIT ?
	`, runID, top)
	if err != nil {
		return nil, fmt.Errorf("reasoning trace: %w", err)
	}
	defer rows.Close()

	var result []string
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		if len(data) > maxChars {
			data = data[:maxChars] + "…"
		}
		result = append(result, data)
	}
	return result, rows.Err()
}

// GetQualityMetricsForReport wraps GetQualityMetrics, returning nil without error when absent.
func (l *LocalDB) GetQualityMetricsForReport(runID string) (*quality.Metrics, error) {
	return l.GetQualityMetrics(runID)
}
