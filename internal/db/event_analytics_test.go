package db

import (
	"encoding/json"
	"testing"
	"time"
)

func insertEvent(t *testing.T, d *LocalDB, runID, eventType string, data map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(data)
	_, err := d.Exec(`
		INSERT INTO events (run_id, layer, event_type, data, ts)
		VALUES (?, 4, ?, ?, ?)
	`, runID, eventType, string(raw), time.Now().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("insert event: %v", err)
	}
}

func seedRunForAnalytics(t *testing.T, d *LocalDB, runID, agent, issue string) {
	t.Helper()
	_, err := d.Exec(`
		INSERT INTO runs (id, jira_issue_key, agent_name, agent_type, user, workstation_id, started_at, status)
		VALUES (?, ?, ?, 'claude_code', 'tester', 'ws-1', ?, 'done')
	`, runID, issue, agent, time.Now().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("seed run: %v", err)
	}
}

// ---- ToolUsage ----

func TestToolUsage_GroupsByTool(t *testing.T) {
	d := setupTestDB(t)
	defer d.Close()
	seedRunForAnalytics(t, d, "r1", "alpha", "K-1")

	insertEvent(t, d, "r1", "tool.use", map[string]any{"tool": "Read"})
	insertEvent(t, d, "r1", "tool.use", map[string]any{"tool": "Read"})
	insertEvent(t, d, "r1", "tool.use", map[string]any{"tool": "Read"})
	insertEvent(t, d, "r1", "tool.use", map[string]any{"tool": "Bash"})
	insertEvent(t, d, "r1", "tool.use", map[string]any{"tool": "Bash"})

	rows, err := d.ToolUsage(0, 10)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	got := map[string]int{}
	for _, r := range rows {
		got[r.Tool] = r.UseCount
	}
	if got["Read"] != 3 || got["Bash"] != 2 {
		t.Errorf("counts=%v, want Read=3, Bash=2", got)
	}
}

func TestToolUsage_SuccessRate(t *testing.T) {
	d := setupTestDB(t)
	defer d.Close()
	seedRunForAnalytics(t, d, "r1", "alpha", "K-1")

	insertEvent(t, d, "r1", "tool.use", map[string]any{"tool": "Bash", "tool_use_id": "u1"})
	insertEvent(t, d, "r1", "tool.use", map[string]any{"tool": "Bash", "tool_use_id": "u2"})
	insertEvent(t, d, "r1", "tool.result", map[string]any{"tool_use_id": "u1", "success": true})
	insertEvent(t, d, "r1", "tool.result", map[string]any{"tool_use_id": "u2", "success": false})

	rows, err := d.ToolUsage(0, 10)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d, want 1 (Bash)", len(rows))
	}
	if rows[0].SuccessRate != 50.0 {
		t.Errorf("success_rate=%.1f, want 50.0", rows[0].SuccessRate)
	}
}

func TestToolUsage_TopK(t *testing.T) {
	d := setupTestDB(t)
	defer d.Close()
	seedRunForAnalytics(t, d, "r1", "alpha", "K-1")

	insertEvent(t, d, "r1", "tool.use", map[string]any{"tool": "Read"})
	insertEvent(t, d, "r1", "tool.use", map[string]any{"tool": "Read"})
	insertEvent(t, d, "r1", "tool.use", map[string]any{"tool": "Bash"})

	rows, err := d.ToolUsage(0, 1)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(rows) != 1 || rows[0].Tool != "Read" {
		t.Errorf("top1=%+v, want Read", rows)
	}
}

func TestToolUsage_Empty(t *testing.T) {
	d := setupTestDB(t)
	defer d.Close()
	rows, err := d.ToolUsage(0, 10)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("got %d, want 0", len(rows))
	}
}

// ---- ContextUsage ----

func TestContextUsage_GroupsByPage(t *testing.T) {
	d := setupTestDB(t)
	defer d.Close()
	seedRunForAnalytics(t, d, "r1", "alpha", "K-1")

	insertEvent(t, d, "r1", "confluence.read", map[string]any{"page_id": "100", "title": "PageA"})
	insertEvent(t, d, "r1", "confluence.read", map[string]any{"page_id": "100", "title": "PageA"})
	insertEvent(t, d, "r1", "confluence.read", map[string]any{"page_id": "200", "title": "PageB"})

	rows, err := d.ContextUsage(0, 10)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d, want 2", len(rows))
	}
	got := map[string]int{}
	titles := map[string]string{}
	for _, r := range rows {
		got[r.PageID] = r.UseCount
		titles[r.PageID] = r.Title
	}
	if got["100"] != 2 || got["200"] != 1 {
		t.Errorf("counts=%v", got)
	}
	if titles["100"] != "PageA" {
		t.Errorf("title for 100=%q", titles["100"])
	}
}

func TestContextUsage_Empty(t *testing.T) {
	d := setupTestDB(t)
	defer d.Close()
	rows, err := d.ContextUsage(0, 10)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("got %d", len(rows))
	}
}

// ---- IterationStats ----

func TestIterationStats_AvgPerAgent(t *testing.T) {
	d := setupTestDB(t)
	defer d.Close()

	// Issue K-1 owned by alpha: 1 iteration event → round count = 2
	seedRunForAnalytics(t, d, "r1", "alpha", "K-1")
	insertEvent(t, d, "r1", "task.iteration.start", map[string]any{"round": 2, "issue_key": "K-1"})

	// Issue K-2 owned by alpha: 0 iteration events → round count = 1
	seedRunForAnalytics(t, d, "r2", "alpha", "K-2")

	// Issue K-3 owned by beta: 2 iteration events → round count = 3
	seedRunForAnalytics(t, d, "r3", "beta", "K-3")
	insertEvent(t, d, "r3", "task.iteration.start", map[string]any{"round": 2, "issue_key": "K-3"})
	insertEvent(t, d, "r3", "task.iteration.start", map[string]any{"round": 3, "issue_key": "K-3"})

	stats, err := d.IterationStats("agent")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	got := map[string]float64{}
	count := map[string]int{}
	for _, s := range stats {
		got[s.GroupKey] = s.AvgRound
		count[s.GroupKey] = s.TaskCount
	}
	// alpha: (2 + 1) / 2 = 1.5
	if got["alpha"] != 1.5 {
		t.Errorf("alpha avg=%.2f, want 1.5", got["alpha"])
	}
	if count["alpha"] != 2 {
		t.Errorf("alpha tasks=%d, want 2", count["alpha"])
	}
	// beta: 3 / 1 = 3
	if got["beta"] != 3.0 {
		t.Errorf("beta avg=%.2f, want 3", got["beta"])
	}
}

func TestIterationStats_Empty(t *testing.T) {
	d := setupTestDB(t)
	defer d.Close()
	stats, err := d.IterationStats("agent")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(stats) != 0 {
		t.Errorf("got %d", len(stats))
	}
}
