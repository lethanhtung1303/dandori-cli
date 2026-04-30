package db

import (
	"testing"
	"time"
)

// seedFullRun inserts a run with all fields needed for incident report queries.
func seedFullRun(t *testing.T, d *LocalDB, runID, jiraKey string, costUSD float64) {
	t.Helper()
	_, err := d.Exec(`
		INSERT INTO runs (
			id, jira_issue_key, agent_name, agent_type, user, workstation_id,
			started_at, ended_at, duration_sec, exit_code, status,
			cost_usd, input_tokens, output_tokens, model,
			git_head_before, git_head_after
		) VALUES (?, ?, 'claude-code', 'claude_code', 'tester', 'ws-1',
			'2026-04-30T10:14:22Z', '2026-04-30T10:16:44Z', 142, 0, 'done',
			?, 10000, 2500, 'claude-sonnet-4',
			'abc1234', 'def5678')
	`, runID, jiraKey, costUSD)
	if err != nil {
		t.Fatalf("seed full run: %v", err)
	}
}

func seedToolUseEvent(t *testing.T, d *LocalDB, runID, tool string) {
	t.Helper()
	_, err := d.Exec(`
		INSERT INTO events (run_id, layer, event_type, data, ts)
		VALUES (?, 3, 'tool.use', json_object('tool', ?), ?)
	`, runID, tool, time.Now().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("seed tool.use: %v", err)
	}
}

// --- GetRunRecord ---

func TestGetRunRecord_Found(t *testing.T) {
	d := setupTestDB(t)
	defer d.Close()
	seedFullRun(t, d, "run-abc", "CLITEST-1", 0.34)

	r, err := d.GetRunRecord("run-abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.ID != "run-abc" {
		t.Errorf("ID=%q want run-abc", r.ID)
	}
	if r.JiraIssueKey != "CLITEST-1" {
		t.Errorf("JiraKey=%q", r.JiraIssueKey)
	}
	if r.AgentName != "claude-code" {
		t.Errorf("AgentName=%q", r.AgentName)
	}
	if r.Status != "done" {
		t.Errorf("Status=%q", r.Status)
	}
	if r.CostUSD != 0.34 {
		t.Errorf("CostUSD=%.2f want 0.34", r.CostUSD)
	}
	if r.GitHeadBefore != "abc1234" {
		t.Errorf("GitHeadBefore=%q", r.GitHeadBefore)
	}
	if r.GitHeadAfter != "def5678" {
		t.Errorf("GitHeadAfter=%q", r.GitHeadAfter)
	}
}

func TestGetRunRecord_NotFound(t *testing.T) {
	d := setupTestDB(t)
	defer d.Close()

	_, err := d.GetRunRecord("does-not-exist")
	if err == nil {
		t.Fatal("expected error for missing run, got nil")
	}
}

// --- GetRunsForJiraKey ---

func TestGetRunsForJiraKey_MultipleRuns(t *testing.T) {
	d := setupTestDB(t)
	defer d.Close()

	seedFullRun(t, d, "run-1", "CLITEST-10", 0.10)
	seedFullRun(t, d, "run-2", "CLITEST-10", 0.20)
	seedFullRun(t, d, "run-3", "CLITEST-99", 0.30) // different key

	runs, err := d.GetRunsForJiraKey("CLITEST-10", time.Time{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("got %d runs, want 2", len(runs))
	}
	// Verify both belong to CLITEST-10
	for _, r := range runs {
		if r.JiraIssueKey != "CLITEST-10" {
			t.Errorf("unexpected jira key %q", r.JiraIssueKey)
		}
	}
}

func TestGetRunsForJiraKey_NotFound(t *testing.T) {
	d := setupTestDB(t)
	defer d.Close()

	runs, err := d.GetRunsForJiraKey("CLITEST-NONE", time.Time{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runs) != 0 {
		t.Errorf("expected 0 runs, got %d", len(runs))
	}
}

func TestGetRunsForJiraKey_SinceFilter(t *testing.T) {
	d := setupTestDB(t)
	defer d.Close()

	// Insert two runs with different timestamps
	_, err := d.Exec(`
		INSERT INTO runs (id, jira_issue_key, agent_name, agent_type, user, workstation_id,
			started_at, status, cost_usd, input_tokens, output_tokens)
		VALUES ('old-run', 'CLITEST-20', 'agent', 'claude_code', 'tester', 'ws-1',
			'2026-01-01T00:00:00Z', 'done', 0.01, 100, 50)
	`)
	if err != nil {
		t.Fatalf("insert old run: %v", err)
	}
	_, err = d.Exec(`
		INSERT INTO runs (id, jira_issue_key, agent_name, agent_type, user, workstation_id,
			started_at, status, cost_usd, input_tokens, output_tokens)
		VALUES ('new-run', 'CLITEST-20', 'agent', 'claude_code', 'tester', 'ws-1',
			'2026-04-01T00:00:00Z', 'done', 0.02, 200, 100)
	`)
	if err != nil {
		t.Fatalf("insert new run: %v", err)
	}

	since := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	runs, err := d.GetRunsForJiraKey("CLITEST-20", since)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("got %d runs with since filter, want 1", len(runs))
	}
	if runs[0].ID != "new-run" {
		t.Errorf("got run %q, want new-run", runs[0].ID)
	}
}

// --- GetToolUsageForRun ---

func TestGetToolUsageForRun_Counts(t *testing.T) {
	d := setupTestDB(t)
	defer d.Close()
	seedFullRun(t, d, "run-tools", "CLITEST-5", 0.01)

	seedToolUseEvent(t, d, "run-tools", "Read")
	seedToolUseEvent(t, d, "run-tools", "Read")
	seedToolUseEvent(t, d, "run-tools", "Read")
	seedToolUseEvent(t, d, "run-tools", "Bash")
	seedToolUseEvent(t, d, "run-tools", "Edit")

	tools, err := d.GetToolUsageForRun("run-tools", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tools) == 0 {
		t.Fatal("expected tool rows, got 0")
	}
	// Read should be first (highest count)
	if tools[0].Tool != "Read" {
		t.Errorf("top tool=%q, want Read", tools[0].Tool)
	}
	if tools[0].Count != 3 {
		t.Errorf("Read count=%d, want 3", tools[0].Count)
	}
}

func TestGetToolUsageForRun_Empty(t *testing.T) {
	d := setupTestDB(t)
	defer d.Close()
	seedFullRun(t, d, "run-empty", "CLITEST-6", 0.01)

	tools, err := d.GetToolUsageForRun("run-empty", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tools) != 0 {
		t.Errorf("expected 0 tools, got %d", len(tools))
	}
}

func TestGetToolUsageForRun_TopLimit(t *testing.T) {
	d := setupTestDB(t)
	defer d.Close()
	seedFullRun(t, d, "run-top", "CLITEST-7", 0.01)

	for _, tool := range []string{"A", "B", "C", "D", "E", "F", "G"} {
		seedToolUseEvent(t, d, "run-top", tool)
	}

	tools, err := d.GetToolUsageForRun("run-top", 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tools) > 3 {
		t.Errorf("got %d tools with top=3", len(tools))
	}
}

// --- GetQualityMetricsForReport ---

func TestGetQualityMetricsForReport_Absent(t *testing.T) {
	d := setupTestDB(t)
	defer d.Close()
	seedFullRun(t, d, "run-noqual", "CLITEST-8", 0.01)

	m, err := d.GetQualityMetricsForReport("run-noqual")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m != nil {
		t.Errorf("expected nil metrics for run with no quality row, got %+v", m)
	}
}
