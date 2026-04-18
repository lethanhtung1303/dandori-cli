package db

import (
	"path/filepath"
	"testing"
)

func TestOpenAndMigrate(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if err := db.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	var tableCount int
	err = db.QueryRow(`
		SELECT COUNT(*) FROM sqlite_master
		WHERE type='table' AND name IN ('runs', 'events', 'audit_log', 'schema_version')
	`).Scan(&tableCount)
	if err != nil {
		t.Fatalf("query tables: %v", err)
	}
	if tableCount != 4 {
		t.Errorf("expected 4 tables, got %d", tableCount)
	}
}

func TestMigrateIdempotent(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	for i := 0; i < 3; i++ {
		if err := db.Migrate(); err != nil {
			t.Fatalf("migrate %d: %v", i, err)
		}
	}

	var version int
	err = db.QueryRow(`SELECT MAX(version) FROM schema_version`).Scan(&version)
	if err != nil {
		t.Fatalf("query version: %v", err)
	}
	if version != SchemaVersion {
		t.Errorf("expected version %d, got %d", SchemaVersion, version)
	}
}

func TestInsertRun(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if err := db.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO runs (id, agent_name, agent_type, user, workstation_id, started_at, status)
		VALUES ('test-run-1', 'alpha', 'claude_code', 'phuc', 'ws-1', datetime('now'), 'running')
	`)
	if err != nil {
		t.Fatalf("insert run: %v", err)
	}

	var count int
	err = db.QueryRow(`SELECT COUNT(*) FROM runs`).Scan(&count)
	if err != nil {
		t.Fatalf("count runs: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 run, got %d", count)
	}
}
