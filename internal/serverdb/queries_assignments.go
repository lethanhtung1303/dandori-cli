package serverdb

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AgentConfigRow struct {
	Name                string
	AgentType           string
	WorkstationID       *string
	Capabilities        []string
	PreferredIssueTypes []string
	MaxConcurrent       int
	Team                *string
	Active              bool
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type AssignmentRow struct {
	ID               int64
	JiraIssueKey     string
	SuggestedAgent   *string
	SuggestedScore   *int
	SuggestionReason *string
	ConfirmedAgent   *string
	Status           string
	SuggestedAt      time.Time
	ConfirmedAt      *time.Time
	ReminderSent     bool
}

func UpsertAgentConfig(ctx context.Context, pool *pgxpool.Pool, cfg AgentConfigRow) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO agent_configs (name, agent_type, workstation_id, capabilities, preferred_issue_types, max_concurrent, team, active, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())
		ON CONFLICT (name) DO UPDATE SET
			agent_type = EXCLUDED.agent_type,
			workstation_id = EXCLUDED.workstation_id,
			capabilities = EXCLUDED.capabilities,
			preferred_issue_types = EXCLUDED.preferred_issue_types,
			max_concurrent = EXCLUDED.max_concurrent,
			team = EXCLUDED.team,
			active = EXCLUDED.active,
			updated_at = now()
	`, cfg.Name, cfg.AgentType, cfg.WorkstationID, cfg.Capabilities, cfg.PreferredIssueTypes, cfg.MaxConcurrent, cfg.Team, cfg.Active)
	return err
}

func ListAgentConfigs(ctx context.Context, pool *pgxpool.Pool, activeOnly bool) ([]AgentConfigRow, error) {
	query := `SELECT name, agent_type, workstation_id, capabilities, preferred_issue_types, max_concurrent, team, active, created_at, updated_at FROM agent_configs`
	if activeOnly {
		query += ` WHERE active = TRUE`
	}
	query += ` ORDER BY name`

	rows, err := pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var configs []AgentConfigRow
	for rows.Next() {
		var c AgentConfigRow
		if err := rows.Scan(&c.Name, &c.AgentType, &c.WorkstationID, &c.Capabilities, &c.PreferredIssueTypes, &c.MaxConcurrent, &c.Team, &c.Active, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		configs = append(configs, c)
	}
	return configs, rows.Err()
}

func GetAgentConfig(ctx context.Context, pool *pgxpool.Pool, name string) (*AgentConfigRow, error) {
	var c AgentConfigRow
	err := pool.QueryRow(ctx, `
		SELECT name, agent_type, workstation_id, capabilities, preferred_issue_types, max_concurrent, team, active, created_at, updated_at
		FROM agent_configs WHERE name = $1
	`, name).Scan(&c.Name, &c.AgentType, &c.WorkstationID, &c.Capabilities, &c.PreferredIssueTypes, &c.MaxConcurrent, &c.Team, &c.Active, &c.CreatedAt, &c.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func CountActiveRuns(ctx context.Context, pool *pgxpool.Pool, agentName string) (int, error) {
	var count int
	err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM runs WHERE agent_name = $1 AND status = 'running'`, agentName).Scan(&count)
	return count, err
}

func CreateAssignment(ctx context.Context, pool *pgxpool.Pool, a AssignmentRow) (int64, error) {
	var id int64
	err := pool.QueryRow(ctx, `
		INSERT INTO assignments (jira_issue_key, suggested_agent, suggested_score, suggestion_reason, status)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, a.JiraIssueKey, a.SuggestedAgent, a.SuggestedScore, a.SuggestionReason, a.Status).Scan(&id)
	return id, err
}

func ConfirmAssignment(ctx context.Context, pool *pgxpool.Pool, id int64, confirmedAgent string) error {
	status := "confirmed"
	if confirmedAgent != "" {
		// Check if it matches suggested
		var suggested *string
		pool.QueryRow(ctx, `SELECT suggested_agent FROM assignments WHERE id = $1`, id).Scan(&suggested)
		if suggested != nil && *suggested != confirmedAgent {
			status = "overridden"
		}
	}

	_, err := pool.Exec(ctx, `
		UPDATE assignments SET confirmed_agent = $1, status = $2, confirmed_at = now()
		WHERE id = $3
	`, confirmedAgent, status, id)
	return err
}

func ListPendingAssignments(ctx context.Context, pool *pgxpool.Pool) ([]AssignmentRow, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, jira_issue_key, suggested_agent, suggested_score, suggestion_reason, confirmed_agent, status, suggested_at, confirmed_at, reminder_sent
		FROM assignments WHERE status = 'pending' ORDER BY suggested_at
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var assignments []AssignmentRow
	for rows.Next() {
		var a AssignmentRow
		if err := rows.Scan(&a.ID, &a.JiraIssueKey, &a.SuggestedAgent, &a.SuggestedScore, &a.SuggestionReason, &a.ConfirmedAgent, &a.Status, &a.SuggestedAt, &a.ConfirmedAt, &a.ReminderSent); err != nil {
			return nil, err
		}
		assignments = append(assignments, a)
	}
	return assignments, rows.Err()
}

func GetAssignmentByIssue(ctx context.Context, pool *pgxpool.Pool, issueKey string) (*AssignmentRow, error) {
	var a AssignmentRow
	err := pool.QueryRow(ctx, `
		SELECT id, jira_issue_key, suggested_agent, suggested_score, suggestion_reason, confirmed_agent, status, suggested_at, confirmed_at, reminder_sent
		FROM assignments WHERE jira_issue_key = $1 ORDER BY suggested_at DESC LIMIT 1
	`, issueKey).Scan(&a.ID, &a.JiraIssueKey, &a.SuggestedAgent, &a.SuggestedScore, &a.SuggestionReason, &a.ConfirmedAgent, &a.Status, &a.SuggestedAt, &a.ConfirmedAt, &a.ReminderSent)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func MarkReminderSent(ctx context.Context, pool *pgxpool.Pool, id int64) error {
	_, err := pool.Exec(ctx, `UPDATE assignments SET reminder_sent = TRUE WHERE id = $1`, id)
	return err
}

func ListAssignmentsForAgent(ctx context.Context, pool *pgxpool.Pool, agentName string, status string) ([]AssignmentRow, error) {
	query := `
		SELECT id, jira_issue_key, suggested_agent, suggested_score, suggestion_reason, confirmed_agent, status, suggested_at, confirmed_at, reminder_sent
		FROM assignments WHERE confirmed_agent = $1`
	args := []any{agentName}

	if status != "" {
		query += ` AND status = $2`
		args = append(args, status)
	}
	query += ` ORDER BY confirmed_at DESC`

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var assignments []AssignmentRow
	for rows.Next() {
		var a AssignmentRow
		if err := rows.Scan(&a.ID, &a.JiraIssueKey, &a.SuggestedAgent, &a.SuggestedScore, &a.SuggestionReason, &a.ConfirmedAgent, &a.Status, &a.SuggestedAt, &a.ConfirmedAt, &a.ReminderSent); err != nil {
			return nil, err
		}
		assignments = append(assignments, a)
	}
	return assignments, rows.Err()
}
