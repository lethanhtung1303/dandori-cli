package db

import (
	"path/filepath"
	"testing"
	"time"
)

func setupTestDB(t *testing.T) *LocalDB {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestGetAgentStats(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Insert test runs
	runs := []struct {
		id, agent string
		exitCode  int
		cost      float64
		tokens    int
	}{
		{"run1", "alpha", 0, 1.50, 5000},
		{"run2", "alpha", 0, 2.00, 6000},
		{"run3", "alpha", 1, 0.50, 2000}, // failed
		{"run4", "beta", 0, 3.00, 8000},
	}

	for _, r := range runs {
		_, err := db.db.Exec(`
			INSERT INTO runs (id, agent_name, agent_type, user, workstation_id,
				started_at, exit_code, status, cost_usd, input_tokens, output_tokens, duration_sec)
			VALUES (?, ?, 'claude', 'test', 'ws1', datetime('now'), ?, ?, ?, ?, ?, 100)
		`, r.id, r.agent, r.exitCode, statusFromCode(r.exitCode), r.cost, r.tokens, r.tokens/3)
		if err != nil {
			t.Fatalf("insert run %s: %v", r.id, err)
		}
	}

	stats, err := db.GetAgentStats()
	if err != nil {
		t.Fatalf("GetAgentStats: %v", err)
	}

	if len(stats) != 2 {
		t.Errorf("expected 2 agents, got %d", len(stats))
	}

	// Alpha should have 3 runs, 66.7% success
	for _, s := range stats {
		t.Logf("Agent %s: %d runs, %.1f%% success, $%.2f",
			s.AgentName, s.RunCount, s.SuccessRate, s.TotalCost)

		if s.AgentName == "alpha" {
			if s.RunCount != 3 {
				t.Errorf("alpha runs = %d, want 3", s.RunCount)
			}
			if s.SuccessRate < 66 || s.SuccessRate > 67 {
				t.Errorf("alpha success = %.1f, want ~66.7", s.SuccessRate)
			}
		}
	}
}

func TestGetCostByAgent(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Insert test data
	_, _ = db.db.Exec(`
		INSERT INTO runs (id, agent_name, agent_type, user, workstation_id, started_at, status, cost_usd, input_tokens, output_tokens)
		VALUES ('r1', 'alpha', 'claude', 'test', 'ws1', datetime('now'), 'done', 2.50, 5000, 1500)
	`)
	_, _ = db.db.Exec(`
		INSERT INTO runs (id, agent_name, agent_type, user, workstation_id, started_at, status, cost_usd, input_tokens, output_tokens)
		VALUES ('r2', 'beta', 'claude', 'test', 'ws1', datetime('now'), 'done', 1.50, 3000, 1000)
	`)

	groups, err := db.GetCostByAgent()
	if err != nil {
		t.Fatalf("GetCostByAgent: %v", err)
	}

	if len(groups) != 2 {
		t.Errorf("expected 2 groups, got %d", len(groups))
	}

	// Should be sorted by cost desc (alpha first)
	if len(groups) > 0 && groups[0].Group != "alpha" {
		t.Errorf("first group = %s, want alpha (highest cost)", groups[0].Group)
	}
}

func TestGetCostByTask(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	_, _ = db.db.Exec(`
		INSERT INTO runs (id, jira_issue_key, agent_name, agent_type, user, workstation_id, started_at, status, cost_usd, input_tokens, output_tokens)
		VALUES ('r1', 'PROJ-1', 'alpha', 'claude', 'test', 'ws1', datetime('now'), 'done', 3.00, 6000, 2000)
	`)
	_, _ = db.db.Exec(`
		INSERT INTO runs (id, jira_issue_key, agent_name, agent_type, user, workstation_id, started_at, status, cost_usd, input_tokens, output_tokens)
		VALUES ('r2', 'PROJ-2', 'alpha', 'claude', 'test', 'ws1', datetime('now'), 'done', 1.00, 2000, 500)
	`)

	groups, err := db.GetCostByTask()
	if err != nil {
		t.Fatalf("GetCostByTask: %v", err)
	}

	if len(groups) != 2 {
		t.Errorf("expected 2 groups, got %d", len(groups))
	}

	for _, g := range groups {
		t.Logf("Task %s: $%.2f, %d runs", g.Group, g.Cost, g.RunCount)
	}
}

func TestGetCostByDay(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	now := time.Now()
	yesterday := now.Add(-24 * time.Hour)

	_, _ = db.db.Exec(`
		INSERT INTO runs (id, agent_name, agent_type, user, workstation_id, started_at, status, cost_usd, input_tokens, output_tokens)
		VALUES ('r1', 'alpha', 'claude', 'test', 'ws1', ?, 'done', 2.00, 4000, 1000)
	`, now.Format(time.RFC3339))
	_, _ = db.db.Exec(`
		INSERT INTO runs (id, agent_name, agent_type, user, workstation_id, started_at, status, cost_usd, input_tokens, output_tokens)
		VALUES ('r2', 'alpha', 'claude', 'test', 'ws1', ?, 'done', 1.50, 3000, 800)
	`, yesterday.Format(time.RFC3339))

	groups, err := db.GetCostByDay()
	if err != nil {
		t.Fatalf("GetCostByDay: %v", err)
	}

	if len(groups) < 1 {
		t.Error("expected at least 1 day group")
	}

	for _, g := range groups {
		t.Logf("Day %s: $%.2f, %d runs", g.Group, g.Cost, g.RunCount)
	}
}

func TestGetRecentRuns(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Insert 5 runs
	for i := 1; i <= 5; i++ {
		_, _ = db.db.Exec(`
			INSERT INTO runs (id, jira_issue_key, agent_name, agent_type, user, workstation_id,
				started_at, status, duration_sec, cost_usd, input_tokens, output_tokens)
			VALUES (?, ?, 'alpha', 'claude', 'test', 'ws1', datetime('now', ?), 'done', 60, 0.50, 1000, 300)
		`, "run-"+string(rune('0'+i)), "PROJ-"+string(rune('0'+i)), "-"+string(rune('0'+i))+" minutes")
	}

	runs, err := db.GetRecentRuns(3)
	if err != nil {
		t.Fatalf("GetRecentRuns: %v", err)
	}

	if len(runs) != 3 {
		t.Errorf("expected 3 runs, got %d", len(runs))
	}

	for _, r := range runs {
		t.Logf("Run %s: %s, %s, $%.2f", r.ID, r.JiraIssueKey, r.Status, r.Cost)
	}
}

func TestGetTotalStats(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Empty DB
	runs, cost, tokens, err := db.GetTotalStats()
	if err != nil {
		t.Fatalf("GetTotalStats empty: %v", err)
	}
	if runs != 0 || cost != 0 || tokens != 0 {
		t.Error("empty DB should return zeros")
	}

	// Add data
	_, _ = db.db.Exec(`
		INSERT INTO runs (id, agent_name, agent_type, user, workstation_id, started_at, status, cost_usd, input_tokens, output_tokens)
		VALUES ('r1', 'alpha', 'claude', 'test', 'ws1', datetime('now'), 'done', 2.50, 5000, 1500)
	`)
	_, _ = db.db.Exec(`
		INSERT INTO runs (id, agent_name, agent_type, user, workstation_id, started_at, status, cost_usd, input_tokens, output_tokens)
		VALUES ('r2', 'beta', 'claude', 'test', 'ws1', datetime('now'), 'done', 1.50, 3000, 1000)
	`)

	runs, cost, tokens, err = db.GetTotalStats()
	if err != nil {
		t.Fatalf("GetTotalStats: %v", err)
	}

	if runs != 2 {
		t.Errorf("runs = %d, want 2", runs)
	}
	if cost != 4.0 {
		t.Errorf("cost = %.2f, want 4.00", cost)
	}
	if tokens != 10500 {
		t.Errorf("tokens = %d, want 10500", tokens)
	}
}

func statusFromCode(code int) string {
	if code == 0 {
		return "done"
	}
	return "failed"
}
