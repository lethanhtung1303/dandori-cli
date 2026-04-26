package db

import (
	"fmt"
	"time"
)

// ToolUsageRow aggregates tool.use / tool.result events per tool name.
type ToolUsageRow struct {
	Tool        string    `json:"tool"`
	UseCount    int       `json:"use_count"`
	SuccessRate float64   `json:"success_rate"` // 0..100; -1 when no tool.result rows yet
	LastUsedAt  time.Time `json:"last_used_at"`
}

// ContextUsageRow aggregates confluence.read events per page.
type ContextUsageRow struct {
	PageID     string    `json:"page_id"`
	Title      string    `json:"title"`
	UseCount   int       `json:"use_count"`
	LastUsedAt time.Time `json:"last_used_at"`
}

// IterationStat aggregates feedback rounds per group dimension.
type IterationStat struct {
	GroupKey  string  `json:"group_key"`
	AvgRound  float64 `json:"avg_round"`
	MaxRound  int     `json:"max_round"`
	TaskCount int     `json:"task_count"`
}

// ToolUsage groups tool.use events by tool name. successRate is computed from
// matching tool.result events via tool_use_id; tools with no recorded result
// rows show -1 (caller renders as "n/a"). sinceDays=0 = no lower bound.
func (l *LocalDB) ToolUsage(sinceDays, top int) ([]ToolUsageRow, error) {
	if top <= 0 {
		top = 50
	}

	useQuery := `
		SELECT json_extract(data, '$.tool') AS tool,
		       COUNT(*) AS use_count,
		       MAX(ts) AS last_used
		FROM events
		WHERE event_type = 'tool.use'
	`
	args := []any{}
	if sinceDays > 0 {
		useQuery += " AND ts >= ?"
		args = append(args, time.Now().AddDate(0, 0, -sinceDays).Format(time.RFC3339))
	}
	useQuery += " GROUP BY tool ORDER BY use_count DESC LIMIT ?"
	args = append(args, top)

	rows, err := l.db.Query(useQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("tool usage query: %w", err)
	}
	defer rows.Close()

	var out []ToolUsageRow
	for rows.Next() {
		var r ToolUsageRow
		var lastUsed string
		if err := rows.Scan(&r.Tool, &r.UseCount, &lastUsed); err != nil {
			return nil, err
		}
		r.LastUsedAt, _ = time.Parse(time.RFC3339, lastUsed)
		r.SuccessRate = -1
		out = append(out, r)
	}

	for i := range out {
		var total, success int
		row := l.db.QueryRow(`
			SELECT
			    COUNT(*) AS total,
			    SUM(CASE WHEN json_extract(data, '$.success') = 1 THEN 1 ELSE 0 END) AS success
			FROM events
			WHERE event_type = 'tool.result'
			  AND json_extract(data, '$.tool_use_id') IN (
			      SELECT json_extract(data, '$.tool_use_id') FROM events
			      WHERE event_type = 'tool.use' AND json_extract(data, '$.tool') = ?
			  )
		`, out[i].Tool)
		if err := row.Scan(&total, &success); err == nil && total > 0 {
			out[i].SuccessRate = float64(success) / float64(total) * 100.0
		}
	}

	return out, nil
}

// ContextUsage groups confluence.read events by page_id.
func (l *LocalDB) ContextUsage(sinceDays, top int) ([]ContextUsageRow, error) {
	if top <= 0 {
		top = 50
	}
	query := `
		SELECT json_extract(data, '$.page_id') AS page_id,
		       json_extract(data, '$.title') AS title,
		       COUNT(*) AS use_count,
		       MAX(ts) AS last_used
		FROM events
		WHERE event_type = 'confluence.read'
	`
	args := []any{}
	if sinceDays > 0 {
		query += " AND ts >= ?"
		args = append(args, time.Now().AddDate(0, 0, -sinceDays).Format(time.RFC3339))
	}
	query += " GROUP BY page_id ORDER BY use_count DESC LIMIT ?"
	args = append(args, top)

	rows, err := l.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("context usage query: %w", err)
	}
	defer rows.Close()

	var out []ContextUsageRow
	for rows.Next() {
		var r ContextUsageRow
		var title *string
		var lastUsed string
		if err := rows.Scan(&r.PageID, &title, &r.UseCount, &lastUsed); err != nil {
			return nil, err
		}
		if title != nil {
			r.Title = *title
		}
		r.LastUsedAt, _ = time.Parse(time.RFC3339, lastUsed)
		out = append(out, r)
	}
	return out, nil
}

// IterationStats groups round counts per task by groupBy dimension and
// returns avg/max/count per group. groupBy: agent | engineer | sprint.
//
// round_count = (number of task.iteration.start events for that issue) + 1.
// The +1 accounts for the implicit first round (no event emitted for round 1).
func (l *LocalDB) IterationStats(groupBy string) ([]IterationStat, error) {
	groupCol := "agent_name"
	switch groupBy {
	case "engineer":
		groupCol = "engineer_name"
	case "sprint":
		groupCol = "jira_sprint_id"
	case "agent", "":
		groupCol = "agent_name"
	default:
		return nil, fmt.Errorf("invalid group: %q (want agent|engineer|sprint)", groupBy)
	}

	// Per-task round counts then aggregate per group. Done in two SQL queries
	// instead of one CTE because SQLite's window-function support is fine
	// here and this stays readable.
	query := fmt.Sprintf(`
		WITH per_task AS (
			SELECT r.jira_issue_key,
			       COALESCE(r.%s, '') AS group_key,
			       1 + (
			           SELECT COUNT(*) FROM events e
			           JOIN runs r2 ON e.run_id = r2.id
			           WHERE r2.jira_issue_key = r.jira_issue_key
			             AND e.event_type = 'task.iteration.start'
			       ) AS round_count
			FROM runs r
			WHERE r.jira_issue_key IS NOT NULL AND r.jira_issue_key != ''
			GROUP BY r.jira_issue_key
		)
		SELECT group_key,
		       AVG(CAST(round_count AS REAL)) AS avg_round,
		       MAX(round_count) AS max_round,
		       COUNT(*) AS task_count
		FROM per_task
		GROUP BY group_key
		ORDER BY avg_round DESC
	`, groupCol)

	rows, err := l.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("iteration stats query: %w", err)
	}
	defer rows.Close()

	var out []IterationStat
	for rows.Next() {
		var s IterationStat
		if err := rows.Scan(&s.GroupKey, &s.AvgRound, &s.MaxRound, &s.TaskCount); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}
