package db

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Scenario 9: DB path with spaces
func TestOpenPathWithSpaces(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "path with spaces", "test.db")
	os.MkdirAll(filepath.Dir(dbPath), 0755)

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("path with spaces should work: %v", err)
	}
	defer db.Close()

	if err := db.Migrate(); err != nil {
		t.Fatalf("migrate failed: %v", err)
	}
}

// Scenario 11: DB file doesn't exist
func TestOpenCreatesNewDB(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "new.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("should create new db: %v", err)
	}
	defer db.Close()

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("db file should be created")
	}
}

// Scenario 12: DB file corrupted
func TestOpenCorruptedDB(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "corrupted.db")
	os.WriteFile(dbPath, []byte("not a sqlite file"), 0644)

	db, err := Open(dbPath)
	if err != nil {
		return // expected
	}
	defer db.Close()

	// Migration should fail on corrupted db
	err = db.Migrate()
	if err == nil {
		t.Error("corrupted db should fail migration")
	}
}

// Scenario 13: Schema already migrated (idempotent)
func TestMigrateIdempotentEdge(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open failed: %v", err)
	}
	defer db.Close()

	// Migrate multiple times
	for i := 0; i < 5; i++ {
		if err := db.Migrate(); err != nil {
			t.Fatalf("migrate %d failed: %v", i, err)
		}
	}

	// Check version is still correct
	var version int
	db.QueryRow(`SELECT MAX(version) FROM schema_version`).Scan(&version)
	if version != SchemaVersion {
		t.Errorf("version = %d, want %d", version, SchemaVersion)
	}
}

// Scenario 15: Concurrent DB writes (WAL mode)
func TestConcurrentWrites(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open failed: %v", err)
	}
	defer db.Close()
	db.Migrate()

	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(n int) {
			_, err := db.Exec(`
				INSERT INTO runs (id, agent_name, agent_type, user, workstation_id, started_at, status)
				VALUES (?, 'agent', 'claude_code', 'user', 'ws', datetime('now'), 'running')
			`, "run-"+string(rune('a'+n)))
			if err != nil {
				t.Errorf("concurrent insert failed: %v", err)
			}
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	var count int
	db.QueryRow(`SELECT COUNT(*) FROM runs`).Scan(&count)
	if count != 10 {
		t.Errorf("expected 10 runs, got %d", count)
	}
}

// Scenario 17: Disk full simulation (using closed connection)
func TestWriteToClosedDB(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open failed: %v", err)
	}
	db.Migrate()
	db.Close()

	// Try to write to closed db
	_, err = db.Exec(`INSERT INTO runs (id, agent_name, agent_type, user, workstation_id, started_at, status)
		VALUES ('test', 'agent', 'claude_code', 'user', 'ws', datetime('now'), 'running')`)
	if err == nil {
		t.Error("closed db should reject writes")
	}
}

// Additional: SQL injection in queries
func TestSQLInjectionPrevention(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open failed: %v", err)
	}
	defer db.Close()
	db.Migrate()

	// Insert with malicious ID
	maliciousID := "'; DROP TABLE runs; --"
	_, err = db.Exec(`
		INSERT INTO runs (id, agent_name, agent_type, user, workstation_id, started_at, status)
		VALUES (?, 'agent', 'claude_code', 'user', 'ws', datetime('now'), 'running')
	`, maliciousID)
	if err != nil {
		t.Fatalf("insert failed: %v", err)
	}

	// Table should still exist
	var count int
	err = db.QueryRow(`SELECT COUNT(*) FROM runs`).Scan(&count)
	if err != nil {
		t.Error("runs table should still exist")
	}
}

// Additional: Unicode data
func TestUnicodeData(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open failed: %v", err)
	}
	defer db.Close()
	db.Migrate()

	unicodeCmd := "claude 修复错误 🔧"
	_, err = db.Exec(`
		INSERT INTO runs (id, agent_name, agent_type, user, workstation_id, cwd, command, started_at, status)
		VALUES ('unicode-test', 'agent', 'claude_code', '用户', 'ws', '/home/用户', ?, datetime('now'), 'running')
	`, unicodeCmd)
	if err != nil {
		t.Fatalf("unicode insert failed: %v", err)
	}

	var cmd string
	db.QueryRow(`SELECT command FROM runs WHERE id = 'unicode-test'`).Scan(&cmd)
	if !strings.Contains(cmd, "修复错误") {
		t.Error("unicode not preserved")
	}
}
