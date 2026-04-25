package db

import (
	"path/filepath"
	"testing"
)

func newEmptyLocalDB(t *testing.T) *LocalDB {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	d, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func columnExists(t *testing.T, d *LocalDB, table, col string) bool {
	t.Helper()
	rows, err := d.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		t.Fatalf("pragma: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notnull, pk int
		var dflt interface{}
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if name == col {
			return true
		}
	}
	return false
}

func TestMigration_V3_AddsEngineerAndDepartment(t *testing.T) {
	d := newEmptyLocalDB(t)

	if err := d.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	if !columnExists(t, d, "runs", "engineer_name") {
		t.Error("engineer_name column missing after migration")
	}
	if !columnExists(t, d, "runs", "department") {
		t.Error("department column missing after migration")
	}
}

func TestMigration_V3_Idempotent(t *testing.T) {
	d := newEmptyLocalDB(t)

	if err := d.Migrate(); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	if err := d.Migrate(); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
}

func TestMigration_V3_PreservesExistingData(t *testing.T) {
	d := newEmptyLocalDB(t)

	if err := d.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	_, err := d.Exec(`INSERT INTO runs (id, agent_name, agent_type, user, workstation_id, started_at, status, cost_usd)
		VALUES ('r-keep', 'alpha', 'claude_code', 'u', 'w', '2026-04-01T00:00:00Z', 'done', 1.50)`)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Re-run — should not wipe data.
	if err := d.Migrate(); err != nil {
		t.Fatalf("second migrate: %v", err)
	}

	var cost float64
	if err := d.QueryRow(`SELECT cost_usd FROM runs WHERE id='r-keep'`).Scan(&cost); err != nil {
		t.Fatalf("select: %v", err)
	}
	if cost != 1.50 {
		t.Errorf("data lost: cost=%v", cost)
	}
}
