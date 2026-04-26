package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/phuc-nt/dandori-cli/internal/db"
)

// setupDashboardDB opens a fresh in-memory (temp-file) SQLite DB for handler tests.
func setupDashboardDB(t *testing.T) *db.LocalDB {
	t.Helper()
	tmpDir := t.TempDir()
	store, err := db.Open(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return store
}

// seedRun inserts a minimal run row into the dashboard test DB.
func seedRun(t *testing.T, store *db.LocalDB, runID, agent, engineer, sprint, issueKey string, costUSD float64) {
	t.Helper()
	_, err := store.Exec(`
		INSERT INTO runs (id, jira_issue_key, jira_sprint_id, agent_name, engineer_name,
		                  agent_type, user, workstation_id, started_at, cost_usd, status)
		VALUES (?, ?, ?, ?, ?, 'claude_code', 'tester', 'ws', ?, ?, 'done')
	`, runID, issueKey, sprint, agent, engineer,
		time.Now().Format(time.RFC3339), costUSD)
	if err != nil {
		t.Fatalf("seedRun %s: %v", runID, err)
	}
}

// seedIteration inserts a task.iteration.start event linked to a run.
func seedIteration(t *testing.T, store *db.LocalDB, runID, issueKey string) {
	t.Helper()
	_, err := store.Exec(`
		INSERT INTO events (run_id, layer, event_type, data, ts)
		VALUES (?, 3, 'task.iteration.start', json_object('issue_key', ?), ?)
	`, runID, issueKey, time.Now().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("seedIteration %s: %v", runID, err)
	}
}

// seedBug inserts a bug.filed event linked to a run.
func seedBug(t *testing.T, store *db.LocalDB, runID, bugKey string) {
	t.Helper()
	_, err := store.Exec(`
		INSERT INTO events (run_id, layer, event_type, data, ts)
		VALUES (?, 3, 'bug.filed', json_object('bug_key', ?), ?)
	`, runID, bugKey, time.Now().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("seedBug %s: %v", runID, err)
	}
}

// get performs a GET against a test server's mux and returns status + body JSON.
func getJSON(t *testing.T, mux *http.ServeMux, path string) (int, []byte) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w.Code, w.Body.Bytes()
}

// ---- Regression Rate ----

func TestQualityRegression_ByAgent(t *testing.T) {
	store := setupDashboardDB(t)
	defer store.Close()

	seedRun(t, store, "r1", "e2e-test-alpha", "Phúc Nguyễn", "SP-1", "TASK-1", 0.30)
	seedRun(t, store, "r2", "e2e-test-alpha", "Phúc Nguyễn", "SP-1", "TASK-2", 0.30)
	seedRun(t, store, "r3", "beta-agent", "Bob", "SP-1", "TASK-3", 0.10)
	seedIteration(t, store, "r1", "TASK-1")

	mux := newDashboardMux(store, "https://jira.example.com")
	status, body := getJSON(t, mux, "/api/quality/regression?by=agent")

	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body: %s", status, body)
	}
	var rows []map[string]any
	if err := json.Unmarshal(body, &rows); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, body)
	}
	if len(rows) == 0 {
		t.Fatal("expected non-empty rows")
	}
}

func TestQualityRegression_ByEngineer(t *testing.T) {
	store := setupDashboardDB(t)
	defer store.Close()

	seedRun(t, store, "r1", "alpha", "Phúc Nguyễn", "SP-1", "TASK-1", 0.30)
	seedRun(t, store, "r2", "alpha", "Phúc Nguyễn", "SP-1", "TASK-2", 0.43)
	seedIteration(t, store, "r1", "TASK-1")

	mux := newDashboardMux(store, "https://jira.example.com")
	status, body := getJSON(t, mux, "/api/quality/regression?by=engineer")

	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body: %s", status, body)
	}
	var rows []map[string]any
	if err := json.Unmarshal(body, &rows); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, body)
	}
	if len(rows) == 0 {
		t.Fatal("expected non-empty rows for engineer")
	}
}

func TestQualityRegression_BySprint(t *testing.T) {
	store := setupDashboardDB(t)
	defer store.Close()

	seedRun(t, store, "r1", "alpha", "Alice", "SP-10", "TASK-1", 0.10)
	seedRun(t, store, "r2", "alpha", "Alice", "SP-10", "TASK-2", 0.10)
	seedIteration(t, store, "r1", "TASK-1")

	mux := newDashboardMux(store, "https://jira.example.com")
	status, body := getJSON(t, mux, "/api/quality/regression?by=sprint")

	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body: %s", status, body)
	}
	var rows []map[string]any
	if err := json.Unmarshal(body, &rows); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, body)
	}
	if len(rows) == 0 {
		t.Fatal("expected non-empty rows for sprint")
	}
}

// ---- Bug Rate ----

func TestQualityBugs_ByAgent(t *testing.T) {
	store := setupDashboardDB(t)
	defer store.Close()

	seedRun(t, store, "r1", "e2e-test-alpha", "Phúc Nguyễn", "SP-1", "TASK-1", 0.30)
	seedRun(t, store, "r2", "e2e-test-alpha", "Phúc Nguyễn", "SP-1", "TASK-2", 0.43)
	seedRun(t, store, "r3", "beta-agent", "Bob", "SP-1", "TASK-3", 0.10)
	seedRun(t, store, "r4", "e2e-test-alpha", "Phúc Nguyễn", "SP-1", "TASK-4", 0.20)
	seedRun(t, store, "r5", "e2e-test-alpha", "Phúc Nguyễn", "SP-1", "TASK-5", 0.20)
	seedRun(t, store, "r6", "e2e-test-alpha", "Phúc Nguyễn", "SP-1", "TASK-6", 0.20)
	seedBug(t, store, "r1", "BUG-A")
	seedBug(t, store, "r2", "BUG-B")

	mux := newDashboardMux(store, "https://jira.example.com")
	status, body := getJSON(t, mux, "/api/quality/bugs?by=agent")

	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body: %s", status, body)
	}
	var rows []map[string]any
	if err := json.Unmarshal(body, &rows); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, body)
	}
	if len(rows) == 0 {
		t.Fatal("expected non-empty rows for bugs by agent")
	}
	// find e2e-test-alpha row
	var alpha map[string]any
	for _, r := range rows {
		if r["group_key"] == "e2e-test-alpha" {
			alpha = r
			break
		}
	}
	if alpha == nil {
		t.Fatalf("e2e-test-alpha not found in rows: %+v", rows)
	}
	if int(alpha["bugs"].(float64)) != 2 {
		t.Errorf("alpha bugs=%v want 2", alpha["bugs"])
	}
	// r1,r2,r4,r5,r6 are e2e-test-alpha; r3 is beta-agent → alpha has 5 runs
	if int(alpha["runs"].(float64)) != 5 {
		t.Errorf("alpha runs=%v want 5", alpha["runs"])
	}
}

func TestQualityBugs_ByEngineer(t *testing.T) {
	store := setupDashboardDB(t)
	defer store.Close()

	seedRun(t, store, "r1", "alpha", "Alice", "SP-1", "TASK-1", 0.10)
	seedRun(t, store, "r2", "alpha", "Bob", "SP-1", "TASK-2", 0.10)
	seedBug(t, store, "r1", "BUG-A")

	mux := newDashboardMux(store, "https://jira.example.com")
	status, body := getJSON(t, mux, "/api/quality/bugs?by=engineer")

	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body: %s", status, body)
	}
	var rows []map[string]any
	if err := json.Unmarshal(body, &rows); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, body)
	}
	if len(rows) == 0 {
		t.Fatal("expected non-empty rows for bugs by engineer")
	}
}

func TestQualityBugs_BySprint(t *testing.T) {
	store := setupDashboardDB(t)
	defer store.Close()

	seedRun(t, store, "r1", "alpha", "Alice", "SP-10", "TASK-1", 0.10)
	seedRun(t, store, "r2", "beta", "Bob", "SP-11", "TASK-2", 0.10)
	seedBug(t, store, "r1", "BUG-A")

	mux := newDashboardMux(store, "https://jira.example.com")
	status, body := getJSON(t, mux, "/api/quality/bugs?by=sprint")

	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body: %s", status, body)
	}
	var rows []map[string]any
	if err := json.Unmarshal(body, &rows); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, body)
	}
	if len(rows) == 0 {
		t.Fatal("expected non-empty rows for bugs by sprint")
	}
}

// ---- Quality-Adjusted Cost ----

func TestQualityCost_ByAgent(t *testing.T) {
	store := setupDashboardDB(t)
	defer store.Close()

	seedRun(t, store, "r1", "e2e-test-alpha", "Phúc Nguyễn", "SP-1", "TASK-1", 0.43)
	seedRun(t, store, "r2", "e2e-test-alpha", "Phúc Nguyễn", "SP-1", "TASK-1", 0.30)

	mux := newDashboardMux(store, "https://jira.example.com")
	status, body := getJSON(t, mux, "/api/quality/cost?by=agent")

	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body: %s", status, body)
	}
	var rows []map[string]any
	if err := json.Unmarshal(body, &rows); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, body)
	}
	if len(rows) == 0 {
		t.Fatal("expected non-empty rows for cost by agent")
	}
	// TASK-1 should have combined cost ~0.73
	var task1 map[string]any
	for _, r := range rows {
		if r["issue_key"] == "TASK-1" {
			task1 = r
			break
		}
	}
	if task1 == nil {
		t.Fatalf("TASK-1 not found in cost rows")
	}
	cost := task1["total_cost_usd"].(float64)
	if cost < 0.72 || cost > 0.74 {
		t.Errorf("TASK-1 cost=%.4f want ~0.73", cost)
	}
}

func TestQualityCost_ByEngineer(t *testing.T) {
	store := setupDashboardDB(t)
	defer store.Close()

	seedRun(t, store, "r1", "alpha", "Phúc Nguyễn", "SP-1", "TASK-1", 0.43)
	seedRun(t, store, "r2", "alpha", "Phúc Nguyễn", "SP-1", "TASK-1", 0.30)

	mux := newDashboardMux(store, "https://jira.example.com")
	status, body := getJSON(t, mux, "/api/quality/cost?by=engineer")

	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body: %s", status, body)
	}
	var rows []map[string]any
	if err := json.Unmarshal(body, &rows); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, body)
	}
	if len(rows) == 0 {
		t.Fatal("expected non-empty rows for cost by engineer")
	}
	if rows[0]["group_key"] != "Phúc Nguyễn" {
		t.Errorf("group_key=%q want 'Phúc Nguyễn'", rows[0]["group_key"])
	}
}

func TestQualityCost_BySprint(t *testing.T) {
	store := setupDashboardDB(t)
	defer store.Close()

	seedRun(t, store, "r1", "alpha", "Alice", "SP-10", "TASK-1", 0.10)
	seedRun(t, store, "r2", "beta", "Bob", "SP-11", "TASK-2", 0.20)

	mux := newDashboardMux(store, "https://jira.example.com")
	status, body := getJSON(t, mux, "/api/quality/cost?by=sprint")

	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body: %s", status, body)
	}
	var rows []map[string]any
	if err := json.Unmarshal(body, &rows); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, body)
	}
	if len(rows) == 0 {
		t.Fatal("expected non-empty rows for cost by sprint")
	}
}

// ---- Edge cases ----

func TestQuality_InvalidBy(t *testing.T) {
	store := setupDashboardDB(t)
	defer store.Close()

	mux := newDashboardMux(store, "https://jira.example.com")
	status, body := getJSON(t, mux, "/api/quality/regression?by=foo")

	if status != http.StatusBadRequest {
		t.Fatalf("status=%d want 400; body: %s", status, body)
	}
	// body should contain "error"
	if len(body) == 0 {
		t.Error("expected non-empty error body")
	}
}

func TestQuality_EmptyDB(t *testing.T) {
	store := setupDashboardDB(t)
	defer store.Close()

	mux := newDashboardMux(store, "https://jira.example.com")
	status, body := getJSON(t, mux, "/api/quality/regression?by=agent")

	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body: %s", status, body)
	}
	// empty DB → null or [] — both are valid; JS handles null with length check
	trimmed := string(body)
	if trimmed != "null\n" && trimmed != "[]\n" {
		// allow null or []
		var rows []map[string]any
		if err := json.Unmarshal(body, &rows); err != nil {
			t.Fatalf("unmarshal empty result: %v; body=%s", err, body)
		}
		if len(rows) != 0 {
			t.Errorf("expected 0 rows for empty DB, got %d", len(rows))
		}
	}
}
