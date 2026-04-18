package watcher

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/phuc-nt/dandori-cli/internal/db"
)

func setupWatcherDB(t *testing.T) *db.LocalDB {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	localDB, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := localDB.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return localDB
}

func writeSessionFile(t *testing.T, dir, sessionID string, lines []string) string {
	path := filepath.Join(dir, sessionID+".jsonl")
	content := ""
	for _, l := range lines {
		content += l + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write session: %v", err)
	}
	return path
}

func TestDiscoverProjects_EmptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	projects, err := DiscoverProjects(tmpDir)
	if err != nil {
		t.Fatalf("DiscoverProjects: %v", err)
	}
	if len(projects) != 0 {
		t.Errorf("projects = %d, want 0", len(projects))
	}
}

func TestDiscoverProjects_FindsDirs(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, "-project-one"), 0755)
	os.MkdirAll(filepath.Join(tmpDir, "-project-two"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "not-a-dir.txt"), []byte(""), 0644)

	projects, err := DiscoverProjects(tmpDir)
	if err != nil {
		t.Fatalf("DiscoverProjects: %v", err)
	}
	if len(projects) != 2 {
		t.Errorf("projects = %d, want 2", len(projects))
	}
}

func TestDiscoverSessions_NoFiles(t *testing.T) {
	tmpDir := t.TempDir()
	sessions, err := DiscoverSessions(tmpDir)
	if err != nil {
		t.Fatalf("DiscoverSessions: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("sessions = %d, want 0", len(sessions))
	}
}

func TestDiscoverSessions_FindsJSONL(t *testing.T) {
	tmpDir := t.TempDir()
	writeSessionFile(t, tmpDir, "abc-123", []string{`{"type":"user"}`})
	writeSessionFile(t, tmpDir, "def-456", []string{`{"type":"assistant"}`})
	os.WriteFile(filepath.Join(tmpDir, "skip.txt"), []byte(""), 0644)

	sessions, err := DiscoverSessions(tmpDir)
	if err != nil {
		t.Fatalf("DiscoverSessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Errorf("sessions = %d, want 2", len(sessions))
	}
}

func TestPollOnce_CreatesOrphanRun(t *testing.T) {
	localDB := setupWatcherDB(t)
	defer localDB.Close()

	claudeDir := t.TempDir()
	projectDir := filepath.Join(claudeDir, "-test-project")
	os.MkdirAll(projectDir, 0755)

	// Write a session with one assistant message with tokens
	writeSessionFile(t, projectDir, "session-xyz",
		[]string{`{"type":"assistant","message":{"model":"claude-sonnet-4-6","usage":{"input_tokens":100,"output_tokens":50}}}`})

	w := New(Config{DB: localDB, ClaudeProjectsRoot: claudeDir})
	if err := w.PollOnce(); err != nil {
		t.Fatalf("PollOnce: %v", err)
	}

	count := countRuns(t, localDB, "session-xyz")
	if count != 1 {
		t.Errorf("expected 1 orphan run for session, got %d", count)
	}
}

func TestPollOnce_Idempotent(t *testing.T) {
	localDB := setupWatcherDB(t)
	defer localDB.Close()

	claudeDir := t.TempDir()
	projectDir := filepath.Join(claudeDir, "-test-project")
	os.MkdirAll(projectDir, 0755)
	writeSessionFile(t, projectDir, "sess-1",
		[]string{`{"type":"assistant","message":{"model":"claude-sonnet-4-6","usage":{"input_tokens":10,"output_tokens":5}}}`})

	w := New(Config{DB: localDB, ClaudeProjectsRoot: claudeDir})
	if err := w.PollOnce(); err != nil {
		t.Fatalf("PollOnce 1: %v", err)
	}
	if err := w.PollOnce(); err != nil {
		t.Fatalf("PollOnce 2: %v", err)
	}

	count := countRuns(t, localDB, "sess-1")
	if count != 1 {
		t.Errorf("expected 1 run after 2 polls, got %d", count)
	}
}

func TestPollOnce_SkipsSessionsAlreadyTrackedByWrapper(t *testing.T) {
	localDB := setupWatcherDB(t)
	defer localDB.Close()

	claudeDir := t.TempDir()
	projectDir := filepath.Join(claudeDir, "-test-project")
	os.MkdirAll(projectDir, 0755)
	writeSessionFile(t, projectDir, "wrapped-session",
		[]string{`{"type":"assistant","message":{"model":"claude-sonnet-4-6","usage":{"input_tokens":20,"output_tokens":10}}}`})

	// Simulate wrapper already stored a run with this session_id
	_, err := localDB.Exec(`
		INSERT INTO runs (id, agent_name, agent_type, user, workstation_id, started_at, status, session_id)
		VALUES (?, 'test', 'claude', 'user', 'ws1', ?, 'done', ?)`,
		"pre-existing", time.Now().Format(time.RFC3339), "wrapped-session")
	if err != nil {
		t.Fatalf("seed pre-existing run: %v", err)
	}

	w := New(Config{DB: localDB, ClaudeProjectsRoot: claudeDir})
	if err := w.PollOnce(); err != nil {
		t.Fatalf("PollOnce: %v", err)
	}

	count := countRuns(t, localDB, "wrapped-session")
	if count != 1 {
		t.Errorf("expected 1 row (wrapper's), got %d", count)
	}
}

func TestPollOnce_ExtractsTokensAndCost(t *testing.T) {
	localDB := setupWatcherDB(t)
	defer localDB.Close()

	claudeDir := t.TempDir()
	projectDir := filepath.Join(claudeDir, "-test-project")
	os.MkdirAll(projectDir, 0755)
	writeSessionFile(t, projectDir, "sess-tokens",
		[]string{`{"type":"assistant","message":{"model":"claude-sonnet-4-6","usage":{"input_tokens":1000,"output_tokens":500}}}`})

	w := New(Config{DB: localDB, ClaudeProjectsRoot: claudeDir})
	if err := w.PollOnce(); err != nil {
		t.Fatalf("PollOnce: %v", err)
	}

	var inTokens, outTokens int
	var cost float64
	var model string
	err := localDB.QueryRow(
		"SELECT input_tokens, output_tokens, cost_usd, model FROM runs WHERE session_id = ?",
		"sess-tokens").Scan(&inTokens, &outTokens, &cost, &model)
	if err != nil {
		t.Fatalf("query run: %v", err)
	}

	if inTokens != 1000 {
		t.Errorf("input_tokens = %d, want 1000", inTokens)
	}
	if outTokens != 500 {
		t.Errorf("output_tokens = %d, want 500", outTokens)
	}
	if cost <= 0 {
		t.Errorf("cost = %f, want > 0", cost)
	}
	if model != "claude-sonnet-4-6" {
		t.Errorf("model = %q, want claude-sonnet-4-6", model)
	}
}

func countRuns(t *testing.T, localDB *db.LocalDB, sessionID string) int {
	var n int
	if err := localDB.QueryRow("SELECT COUNT(*) FROM runs WHERE session_id = ?", sessionID).Scan(&n); err != nil {
		t.Fatalf("count runs: %v", err)
	}
	return n
}
