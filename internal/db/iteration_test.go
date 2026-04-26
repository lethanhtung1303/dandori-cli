package db

import (
	"testing"
	"time"
)

func insertRunForIteration(t *testing.T, d *LocalDB, runID, issueKey, status string, startedAt time.Time) {
	t.Helper()
	_, err := d.Exec(`
		INSERT INTO runs (id, jira_issue_key, agent_type, user, workstation_id, started_at, status)
		VALUES (?, ?, 'claude_code', 'tester', 'ws-1', ?, ?)
	`, runID, issueKey, startedAt.Format(time.RFC3339), status)
	if err != nil {
		t.Fatalf("insert run: %v", err)
	}
}

func insertIterationEvent(t *testing.T, d *LocalDB, runID, dataJSON string) {
	t.Helper()
	_, err := d.Exec(`
		INSERT INTO events (run_id, layer, event_type, data, ts)
		VALUES (?, 4, 'task.iteration.start', ?, ?)
	`, runID, dataJSON, time.Now().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("insert event: %v", err)
	}
}

func TestLatestRunForIssue(t *testing.T) {
	d := setupTestDB(t)
	defer d.Close()

	t0 := time.Date(2026, 4, 20, 9, 0, 0, 0, time.UTC)
	insertRunForIteration(t, d, "r1", "KEY-1", "done", t0)
	insertRunForIteration(t, d, "r2", "KEY-1", "done", t0.Add(2*time.Hour))
	insertRunForIteration(t, d, "r3", "KEY-1", "done", t0.Add(4*time.Hour))
	insertRunForIteration(t, d, "rX", "OTHER-99", "done", t0.Add(time.Hour))

	got, err := d.LatestRunForIssue("KEY-1")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got == nil {
		t.Fatal("got nil, want r3")
	}
	if got.ID != "r3" {
		t.Errorf("id=%q, want r3", got.ID)
	}
	if got.Status != "done" {
		t.Errorf("status=%q", got.Status)
	}
}

func TestLatestRunForIssue_None(t *testing.T) {
	d := setupTestDB(t)
	defer d.Close()
	got, err := d.LatestRunForIssue("NOPE-1")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != nil {
		t.Errorf("got %+v, want nil", got)
	}
}

func TestIterationEventsForIssue(t *testing.T) {
	d := setupTestDB(t)
	defer d.Close()

	t0 := time.Date(2026, 4, 20, 9, 0, 0, 0, time.UTC)
	insertRunForIteration(t, d, "r1", "KEY-1", "done", t0)
	insertRunForIteration(t, d, "r2", "KEY-1", "done", t0.Add(2*time.Hour))
	insertIterationEvent(t, d, "r2", `{"round":2,"issue_key":"KEY-1","transitioned_at":"2026-04-21T08:00:00Z"}`)
	insertIterationEvent(t, d, "r2", `{"round":3,"issue_key":"KEY-1","transitioned_at":"2026-04-22T08:00:00Z"}`)

	events, err := d.IterationEventsForIssue("KEY-1")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}

	rounds := map[int]bool{events[0].Round: true, events[1].Round: true}
	if !rounds[2] || !rounds[3] {
		t.Errorf("rounds=%v, want {2,3}", rounds)
	}
}

func TestIterationEventsForIssue_Empty(t *testing.T) {
	d := setupTestDB(t)
	defer d.Close()
	events, err := d.IterationEventsForIssue("NOPE-1")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("got %d, want 0", len(events))
	}
}
