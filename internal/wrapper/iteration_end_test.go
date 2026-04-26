package wrapper

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/phuc-nt/dandori-cli/internal/db"
)

func openDBForIterationEnd(t *testing.T) *db.LocalDB {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	if err := d.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return d
}

func seedRunAndStart(t *testing.T, d *db.LocalDB, runID string, round int, issueKey string) {
	t.Helper()
	_, err := d.Exec(`
		INSERT INTO runs (id, jira_issue_key, agent_type, user, workstation_id, started_at, status)
		VALUES (?, ?, 'claude_code', 'tester', 'ws-1', ?, 'done')
	`, runID, issueKey, time.Now().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("seed run: %v", err)
	}
	payload, _ := json.Marshal(map[string]any{
		"round":           round,
		"issue_key":       issueKey,
		"transitioned_at": time.Now().Format(time.RFC3339),
	})
	_, err = d.Exec(`
		INSERT INTO events (run_id, layer, event_type, data, ts)
		VALUES (?, 4, 'task.iteration.start', ?, ?)
	`, runID, string(payload), time.Now().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("seed start event: %v", err)
	}
}

func TestEmitIterationEnd_HasMatchingStart_EmitsEnd(t *testing.T) {
	d := openDBForIterationEnd(t)
	seedRunAndStart(t, d, "run-1", 2, "KEY-1")

	emitIterationEndIfApplicable(d, "run-1")

	var data string
	err := d.QueryRow(`
		SELECT data FROM events
		WHERE run_id=? AND event_type='task.iteration.end'
	`, "run-1").Scan(&data)
	if err != nil {
		t.Fatalf("expected end event: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		t.Fatalf("parse: %v", err)
	}
	// JSON unmarshals numbers to float64.
	if round, _ := payload["round"].(float64); round != 2 {
		t.Errorf("round=%v, want 2", payload["round"])
	}
	if payload["issue_key"] != "KEY-1" {
		t.Errorf("issue_key=%v", payload["issue_key"])
	}
	if payload["ended_at"] == nil || payload["ended_at"] == "" {
		t.Errorf("missing ended_at: %v", payload["ended_at"])
	}
}

func TestEmitIterationEnd_NoStart_NoOp(t *testing.T) {
	d := openDBForIterationEnd(t)
	// Seed run but no start event — round 1 implicit.
	_, err := d.Exec(`
		INSERT INTO runs (id, agent_type, user, workstation_id, started_at, status)
		VALUES ('run-1', 'claude_code', 'tester', 'ws-1', ?, 'done')
	`, time.Now().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	emitIterationEndIfApplicable(d, "run-1")

	var n int
	if err := d.QueryRow(`
		SELECT COUNT(*) FROM events WHERE run_id='run-1' AND event_type='task.iteration.end'
	`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Errorf("end events=%d, want 0", n)
	}
}
