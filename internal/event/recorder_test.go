package event

import (
	"path/filepath"
	"testing"

	"github.com/phuc-nt/dandori-cli/internal/db"
	"github.com/phuc-nt/dandori-cli/internal/model"
)

func setupTestDB(t *testing.T) *db.LocalDB {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	localDB, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	if err := localDB.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	_, err = localDB.Exec(`
		INSERT INTO runs (id, agent_name, agent_type, user, workstation_id, started_at, status)
		VALUES ('test-run-1', 'alpha', 'claude_code', 'phuc', 'ws-1', datetime('now'), 'running')
	`)
	if err != nil {
		t.Fatalf("insert run: %v", err)
	}

	return localDB
}

func TestRecordEvent(t *testing.T) {
	localDB := setupTestDB(t)
	defer localDB.Close()

	recorder := NewRecorder(localDB)

	err := recorder.RecordEvent("test-run-1", model.LayerSkill, "decision", map[string]string{
		"rationale": "chose REST over GraphQL",
	})
	if err != nil {
		t.Fatalf("record event: %v", err)
	}

	var count int
	localDB.QueryRow(`SELECT COUNT(*) FROM events WHERE run_id = 'test-run-1'`).Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 event, got %d", count)
	}

	var eventType string
	var layer int
	localDB.QueryRow(`SELECT event_type, layer FROM events WHERE run_id = 'test-run-1'`).Scan(&eventType, &layer)
	if eventType != "decision" {
		t.Errorf("expected event_type 'decision', got %s", eventType)
	}
	if layer != int(model.LayerSkill) {
		t.Errorf("expected layer %d, got %d", model.LayerSkill, layer)
	}
}

func TestRecordMultipleEvents(t *testing.T) {
	localDB := setupTestDB(t)
	defer localDB.Close()

	recorder := NewRecorder(localDB)

	events := []struct {
		layer     model.EventLayer
		eventType string
		data      any
	}{
		{model.LayerWrapper, "run_started", map[string]string{"command": "claude fix bug"}},
		{model.LayerTailer, "tokens_updated", map[string]int{"input": 1000, "output": 500}},
		{model.LayerSkill, "file_change", map[string][]string{"files": {"src/main.go"}}},
	}

	for _, e := range events {
		if err := recorder.RecordEvent("test-run-1", e.layer, e.eventType, e.data); err != nil {
			t.Fatalf("record event %s: %v", e.eventType, err)
		}
	}

	var count int
	localDB.QueryRow(`SELECT COUNT(*) FROM events WHERE run_id = 'test-run-1'`).Scan(&count)
	if count != 3 {
		t.Errorf("expected 3 events, got %d", count)
	}
}

func TestRecordAuditEvent(t *testing.T) {
	localDB := setupTestDB(t)
	defer localDB.Close()

	recorder := NewRecorder(localDB)

	err := recorder.RecordAuditEvent("phuc", model.AuditRunStarted, "run", "test-run-1", map[string]string{
		"command": "claude fix auth",
	})
	if err != nil {
		t.Fatalf("record audit event: %v", err)
	}

	var count int
	localDB.QueryRow(`SELECT COUNT(*) FROM audit_log`).Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 audit event, got %d", count)
	}

	var currHash, prevHash string
	localDB.QueryRow(`SELECT curr_hash, prev_hash FROM audit_log ORDER BY id DESC LIMIT 1`).Scan(&currHash, &prevHash)
	if currHash == "" {
		t.Error("curr_hash should not be empty")
	}
	if prevHash != "" {
		t.Error("first event prev_hash should be empty")
	}
}

func TestAuditEventHashChain(t *testing.T) {
	localDB := setupTestDB(t)
	defer localDB.Close()

	recorder := NewRecorder(localDB)

	recorder.RecordAuditEvent("phuc", model.AuditRunStarted, "run", "test-run-1", nil)
	recorder.RecordAuditEvent("phuc", model.AuditRunCompleted, "run", "test-run-1", nil)
	recorder.RecordAuditEvent("phuc", model.AuditConfigChanged, "config", "agent", nil)

	rows, _ := localDB.Query(`SELECT prev_hash, curr_hash FROM audit_log ORDER BY id`)
	defer rows.Close()

	var hashes []struct{ prev, curr string }
	for rows.Next() {
		var h struct{ prev, curr string }
		rows.Scan(&h.prev, &h.curr)
		hashes = append(hashes, h)
	}

	if len(hashes) != 3 {
		t.Fatalf("expected 3 audit events, got %d", len(hashes))
	}

	if hashes[0].prev != "" {
		t.Error("first event prev_hash should be empty")
	}

	if hashes[1].prev != hashes[0].curr {
		t.Error("second event prev_hash should equal first event curr_hash")
	}

	if hashes[2].prev != hashes[1].curr {
		t.Error("third event prev_hash should equal second event curr_hash")
	}
}

func TestGetUnsyncedEvents(t *testing.T) {
	localDB := setupTestDB(t)
	defer localDB.Close()

	recorder := NewRecorder(localDB)

	for i := 0; i < 5; i++ {
		recorder.RecordEvent("test-run-1", model.LayerSkill, "test", map[string]int{"i": i})
	}

	events, err := recorder.GetUnsyncedEvents(10)
	if err != nil {
		t.Fatalf("get unsynced events: %v", err)
	}

	if len(events) != 5 {
		t.Errorf("expected 5 unsynced events, got %d", len(events))
	}

	for _, e := range events {
		if e.Synced {
			t.Error("event should not be synced")
		}
	}
}

func TestMarkEventsSynced(t *testing.T) {
	localDB := setupTestDB(t)
	defer localDB.Close()

	recorder := NewRecorder(localDB)

	for i := 0; i < 3; i++ {
		recorder.RecordEvent("test-run-1", model.LayerSkill, "test", nil)
	}

	events, _ := recorder.GetUnsyncedEvents(10)
	ids := make([]int64, len(events))
	for i, e := range events {
		ids[i] = e.ID
	}

	if err := recorder.MarkEventsSynced(ids[:2]); err != nil {
		t.Fatalf("mark events synced: %v", err)
	}

	remaining, _ := recorder.GetUnsyncedEvents(10)
	if len(remaining) != 1 {
		t.Errorf("expected 1 unsynced event remaining, got %d", len(remaining))
	}
}
