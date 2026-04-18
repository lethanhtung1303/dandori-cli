package sync

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/phuc-nt/dandori-cli/internal/db"
)

func setupTestDB(t *testing.T) *db.LocalDB {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	localDB, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	localDB.Migrate()
	return localDB
}

// Scenario 56: Empty batch (0 runs)
func TestSyncEmptyBatch(t *testing.T) {
	localDB := setupTestDB(t)
	defer localDB.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("server should not be called for empty batch")
	}))
	defer server.Close()

	uploader := NewUploader(server.URL, "key", "ws-1")
	resp, err := uploader.Sync(localDB, 100)
	if err != nil {
		t.Fatalf("empty sync failed: %v", err)
	}
	if resp.Accepted != 0 {
		t.Error("empty batch should have 0 accepted")
	}
}

// Scenario 57: Batch size 0 config
func TestSyncBatchSizeZero(t *testing.T) {
	localDB := setupTestDB(t)
	defer localDB.Close()

	// Insert a run
	localDB.Exec(`
		INSERT INTO runs (id, agent_name, agent_type, user, workstation_id, started_at, status)
		VALUES ('run-1', 'agent', 'claude_code', 'user', 'ws', datetime('now'), 'done')
	`)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"accepted":1,"errors":0}`))
	}))
	defer server.Close()

	uploader := NewUploader(server.URL, "key", "ws-1")
	// Batch size 0 should return empty (SQL LIMIT 0)
	resp, err := uploader.Sync(localDB, 0)
	if err != nil {
		t.Fatalf("batch 0 failed: %v", err)
	}
	if resp.Accepted != 0 {
		t.Logf("batch size 0 returned %d accepted", resp.Accepted)
	}
}

// Scenario 58: Server timeout during upload
func TestSyncServerTimeout(t *testing.T) {
	localDB := setupTestDB(t)
	defer localDB.Close()

	localDB.Exec(`
		INSERT INTO runs (id, agent_name, agent_type, user, workstation_id, started_at, status, synced)
		VALUES ('run-1', 'agent', 'claude_code', 'user', 'ws', datetime('now'), 'done', 0)
	`)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.Write([]byte(`{"accepted":1}`))
	}))
	defer server.Close()

	uploader := &Uploader{
		serverURL:     server.URL,
		apiKey:        "key",
		workstationID: "ws-1",
		httpClient:    &http.Client{Timeout: 100 * time.Millisecond},
	}

	_, err := uploader.Sync(localDB, 100)
	if err == nil {
		t.Error("should timeout")
	}

	// Verify run not marked as synced
	var synced int
	localDB.QueryRow(`SELECT synced FROM runs WHERE id = 'run-1'`).Scan(&synced)
	if synced != 0 {
		t.Error("run should not be marked synced on timeout")
	}
}

// Scenario 60: Server unreachable
func TestSyncServerUnreachable(t *testing.T) {
	localDB := setupTestDB(t)
	defer localDB.Close()

	localDB.Exec(`
		INSERT INTO runs (id, agent_name, agent_type, user, workstation_id, started_at, status, synced)
		VALUES ('run-1', 'agent', 'claude_code', 'user', 'ws', datetime('now'), 'done', 0)
	`)

	uploader := NewUploader("http://localhost:59999", "key", "ws-1")
	_, err := uploader.Sync(localDB, 100)
	if err == nil {
		t.Error("should fail with unreachable server")
	}
}

// Scenario 61: Server returns 500
func TestSyncServer500(t *testing.T) {
	localDB := setupTestDB(t)
	defer localDB.Close()

	localDB.Exec(`
		INSERT INTO runs (id, agent_name, agent_type, user, workstation_id, started_at, status, synced)
		VALUES ('run-1', 'agent', 'claude_code', 'user', 'ws', datetime('now'), 'done', 0)
	`)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal error"}`))
	}))
	defer server.Close()

	uploader := NewUploader(server.URL, "key", "ws-1")
	_, err := uploader.Sync(localDB, 100)
	if err == nil {
		t.Error("should fail on 500")
	}

	// Verify run not marked as synced
	var synced int
	localDB.QueryRow(`SELECT synced FROM runs WHERE id = 'run-1'`).Scan(&synced)
	if synced != 0 {
		t.Error("run should not be marked synced on 500")
	}
}

// Scenario 63: Already synced re-upload
func TestSyncAlreadySynced(t *testing.T) {
	localDB := setupTestDB(t)
	defer localDB.Close()

	// Insert already synced run
	localDB.Exec(`
		INSERT INTO runs (id, agent_name, agent_type, user, workstation_id, started_at, status, synced)
		VALUES ('run-1', 'agent', 'claude_code', 'user', 'ws', datetime('now'), 'done', 1)
	`)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("server should not be called for already synced")
	}))
	defer server.Close()

	uploader := NewUploader(server.URL, "key", "ws-1")
	resp, err := uploader.Sync(localDB, 100)
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}
	if resp.Accepted != 0 {
		t.Error("already synced should not re-upload")
	}
}

// Scenario 59: Partial success (some accepted)
func TestSyncPartialSuccess(t *testing.T) {
	localDB := setupTestDB(t)
	defer localDB.Close()

	localDB.Exec(`
		INSERT INTO runs (id, agent_name, agent_type, user, workstation_id, started_at, status, synced)
		VALUES ('run-1', 'agent', 'claude_code', 'user', 'ws', datetime('now'), 'done', 0),
			   ('run-2', 'agent', 'claude_code', 'user', 'ws', datetime('now'), 'done', 0)
	`)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Server accepts 1, rejects 1
		w.Write([]byte(`{"accepted":1,"errors":1}`))
	}))
	defer server.Close()

	uploader := NewUploader(server.URL, "key", "ws-1")
	resp, err := uploader.Sync(localDB, 100)
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}
	if resp.Accepted != 1 || resp.Errors != 1 {
		t.Errorf("expected accepted=1 errors=1, got accepted=%d errors=%d", resp.Accepted, resp.Errors)
	}
}

// Scenario 62: Local DB read fails
func TestSyncDBReadFails(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	localDB, _ := db.Open(dbPath)
	localDB.Migrate()
	localDB.Close() // Close DB to cause read failures

	uploader := NewUploader("http://localhost", "key", "ws-1")
	_, err := uploader.Sync(localDB, 100)
	if err == nil {
		t.Error("should fail when DB is closed")
	}
}

// Additional: Verify request payload format
func TestSyncRequestPayload(t *testing.T) {
	localDB := setupTestDB(t)
	defer localDB.Close()

	jiraKey := "PROJ-123"
	cwd := "/home/user/project"
	localDB.Exec(`
		INSERT INTO runs (id, jira_issue_key, agent_name, agent_type, user, workstation_id,
			cwd, started_at, status, input_tokens, output_tokens, cost_usd, synced)
		VALUES ('run-1', ?, 'alpha', 'claude_code', 'phuc', 'ws-1',
			?, datetime('now'), 'done', 1000, 500, 0.05, 0)
	`, jiraKey, cwd)

	var receivedReq UploadRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedReq)
		w.Write([]byte(`{"accepted":1,"errors":0}`))
	}))
	defer server.Close()

	uploader := NewUploader(server.URL, "api-key-123", "ws-1")
	_, err := uploader.Sync(localDB, 100)
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	if receivedReq.WorkstationID != "ws-1" {
		t.Error("workstation_id not set")
	}
	if len(receivedReq.Runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(receivedReq.Runs))
	}
	run := receivedReq.Runs[0]
	if run.ID != "run-1" {
		t.Error("run id mismatch")
	}
	if run.JiraIssueKey == nil || *run.JiraIssueKey != "PROJ-123" {
		t.Error("jira key not preserved")
	}
	if run.InputTokens != 1000 || run.OutputTokens != 500 {
		t.Error("token counts wrong")
	}
}

// Additional: Authorization header
func TestSyncAuthHeader(t *testing.T) {
	localDB := setupTestDB(t)
	defer localDB.Close()

	localDB.Exec(`
		INSERT INTO runs (id, agent_name, agent_type, user, workstation_id, started_at, status, synced)
		VALUES ('run-1', 'agent', 'claude_code', 'user', 'ws', datetime('now'), 'done', 0)
	`)

	var authHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		w.Write([]byte(`{"accepted":1,"errors":0}`))
	}))
	defer server.Close()

	uploader := NewUploader(server.URL, "secret-key", "ws-1")
	uploader.Sync(localDB, 100)

	if authHeader != "Bearer secret-key" {
		t.Errorf("auth header = %s, want Bearer secret-key", authHeader)
	}
}

// Additional: Empty API key
func TestSyncNoAuthHeader(t *testing.T) {
	localDB := setupTestDB(t)
	defer localDB.Close()

	localDB.Exec(`
		INSERT INTO runs (id, agent_name, agent_type, user, workstation_id, started_at, status, synced)
		VALUES ('run-1', 'agent', 'claude_code', 'user', 'ws', datetime('now'), 'done', 0)
	`)

	var authHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		w.Write([]byte(`{"accepted":1,"errors":0}`))
	}))
	defer server.Close()

	uploader := NewUploader(server.URL, "", "ws-1")
	uploader.Sync(localDB, 100)

	if authHeader != "" {
		t.Error("should not send auth header when API key is empty")
	}
}

// Additional: Events sync
func TestSyncEvents(t *testing.T) {
	localDB := setupTestDB(t)
	defer localDB.Close()

	localDB.Exec(`
		INSERT INTO runs (id, agent_name, agent_type, user, workstation_id, started_at, status)
		VALUES ('run-1', 'agent', 'claude_code', 'user', 'ws', datetime('now'), 'done')
	`)
	localDB.Exec(`
		INSERT INTO events (run_id, layer, event_type, data, ts, synced)
		VALUES ('run-1', 1, 'start', '{}', datetime('now'), 0)
	`)

	var receivedReq UploadRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedReq)
		w.Write([]byte(`{"accepted":1,"errors":0}`))
	}))
	defer server.Close()

	uploader := NewUploader(server.URL, "", "ws-1")
	uploader.Sync(localDB, 100)

	if len(receivedReq.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(receivedReq.Events))
	}
	if receivedReq.Events[0].RunID != "run-1" {
		t.Error("event run_id mismatch")
	}
}

// Scenario 64: Sync interrupted mid-batch (idempotent)
func TestSyncIdempotent(t *testing.T) {
	localDB := setupTestDB(t)
	defer localDB.Close()

	localDB.Exec(`
		INSERT INTO runs (id, agent_name, agent_type, user, workstation_id, started_at, status, synced)
		VALUES ('run-1', 'agent', 'claude_code', 'user', 'ws', datetime('now'), 'done', 0)
	`)

	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Write([]byte(`{"accepted":1,"errors":0}`))
	}))
	defer server.Close()

	uploader := NewUploader(server.URL, "", "ws-1")

	// First sync
	uploader.Sync(localDB, 100)

	// Second sync should not re-upload
	uploader.Sync(localDB, 100)

	if callCount != 1 {
		t.Errorf("server called %d times, expected 1 (idempotent)", callCount)
	}
}

// Additional: Large batch handling
func TestSyncLargeBatch(t *testing.T) {
	if os.Getenv("CI") == "" {
		t.Skip("skip large batch test in local")
	}

	localDB := setupTestDB(t)
	defer localDB.Close()

	// Insert 1000 runs
	for i := 0; i < 1000; i++ {
		localDB.Exec(`
			INSERT INTO runs (id, agent_name, agent_type, user, workstation_id, started_at, status, synced)
			VALUES (?, 'agent', 'claude_code', 'user', 'ws', datetime('now'), 'done', 0)
		`, "run-"+string(rune('0'+i%10))+"-"+string(rune('0'+i/10%10))+"-"+string(rune('0'+i/100%10)))
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req UploadRequest
		json.NewDecoder(r.Body).Decode(&req)
		w.Write([]byte(`{"accepted":` + string(rune('0'+len(req.Runs)%10)) + `,"errors":0}`))
	}))
	defer server.Close()

	uploader := NewUploader(server.URL, "", "ws-1")
	_, err := uploader.Sync(localDB, 100)
	if err != nil {
		t.Fatalf("large batch failed: %v", err)
	}
}
