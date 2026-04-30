package cmd

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/phuc-nt/dandori-cli/internal/db"
)

// --- helpers ----------------------------------------------------------------

func setupIntentTestDB(t *testing.T) *db.LocalDB {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return store
}

func insertRun(t *testing.T, store *db.LocalDB, runID string) {
	t.Helper()
	_, err := store.Exec(`
		INSERT INTO runs (id, jira_issue_key, agent_name, agent_type, user, workstation_id, started_at, status)
		VALUES (?, 'CLITEST-1', 'claude-code', 'claude_code', 'tester', 'ws-1', ?, 'done')
	`, runID, time.Now().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("insert run: %v", err)
	}
}

func insertLayerEvent(t *testing.T, store *db.LocalDB, runID, eventType string, data map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(data)
	_, err := store.Exec(`
		INSERT INTO events (run_id, layer, event_type, data, ts)
		VALUES (?, 4, ?, ?, ?)
	`, runID, eventType, string(raw), time.Now().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("insert event: %v", err)
	}
}

func baseRun(id string) db.SyncableRun {
	return db.SyncableRun{
		ID:           id,
		JiraIssueKey: "CLITEST-1",
		AgentName:    "claude-code",
		Status:       "done",
		Duration:     142,
		Cost:         0.34,
		Tokens:       12500,
	}
}

// --- truncateJira unit tests ------------------------------------------------

func TestTruncateJira_WithinLimit(t *testing.T) {
	s := "hello"
	if got := truncateJira(s, 10); got != s {
		t.Errorf("got %q, want %q", got, s)
	}
}

func TestTruncateJira_ExactLimit(t *testing.T) {
	s := "hello"
	if got := truncateJira(s, 5); got != s {
		t.Errorf("got %q, want %q", got, s)
	}
}

func TestTruncateJira_Truncated(t *testing.T) {
	s := "hello world"
	got := truncateJira(s, 5)
	if !strings.HasSuffix(got, "…") {
		t.Errorf("expected ellipsis suffix, got %q", got)
	}
	if strings.Contains(got, "world") {
		t.Errorf("should not contain truncated part, got %q", got)
	}
}

func TestTruncateJira_Unicode(t *testing.T) {
	// "日本語" = 3 runes; cap at 2 should truncate.
	s := "日本語"
	got := truncateJira(s, 2)
	if strings.Contains(got, "語") {
		t.Errorf("third rune should be truncated, got %q", got)
	}
}

// --- formatRunCommentWithStore table tests ----------------------------------

func TestFormatRunComment_LegacyNoStore(t *testing.T) {
	run := baseRun("r-legacy")
	comment := formatRunComment(run)

	wantParts := []string{
		"✅ *Agent Run Completed*",
		"*Agent:* claude-code",
		"*Duration:* 142s",
		"*Cost:* $0.34",
		"*Tokens:* 12500",
		"_Task completed by AI agent._",
	}
	for _, want := range wantParts {
		if !strings.Contains(comment, want) {
			t.Errorf("missing %q in comment:\n%s", want, comment)
		}
	}
	// Must NOT contain intent sections.
	if strings.Contains(comment, "h3. Intent") {
		t.Errorf("legacy path should not have Intent section:\n%s", comment)
	}
}

func TestFormatRunComment_NoIntentEvents(t *testing.T) {
	store := setupIntentTestDB(t)
	defer store.Close()

	run := baseRun("r-no-intent")
	insertRun(t, store, run.ID)

	comment := formatRunCommentWithStore(run, store)

	if strings.Contains(comment, "h3. Intent") {
		t.Errorf("should not have Intent section when no events:\n%s", comment)
	}
	if strings.Contains(comment, "h3. Key Decisions") {
		t.Errorf("should not have Key Decisions section when no events:\n%s", comment)
	}
	// Base fields must still be present.
	if !strings.Contains(comment, "*Agent:* claude-code") {
		t.Errorf("base field missing:\n%s", comment)
	}
}

func TestFormatRunComment_WithIntent_NoDecisions(t *testing.T) {
	store := setupIntentTestDB(t)
	defer store.Close()

	run := baseRun("r-intent-only")
	insertRun(t, store, run.ID)

	insertLayerEvent(t, store, run.ID, "intent.extracted", map[string]any{
		"first_user_msg": "Fix the login bug",
		"summary":        "Patched auth middleware to handle expired sessions",
		"spec_links": map[string]any{
			"jira_key":        "CLITEST-1",
			"confluence_urls": []string{},
			"source_paths":    []string{},
		},
	})

	comment := formatRunCommentWithStore(run, store)

	if !strings.Contains(comment, "h3. Intent") {
		t.Errorf("missing Intent section:\n%s", comment)
	}
	if !strings.Contains(comment, "Fix the login bug") {
		t.Errorf("missing first_user_msg:\n%s", comment)
	}
	if !strings.Contains(comment, "Patched auth middleware") {
		t.Errorf("missing summary:\n%s", comment)
	}
	if strings.Contains(comment, "h3. Key Decisions") {
		t.Errorf("should not have Key Decisions when none present:\n%s", comment)
	}
}

func TestFormatRunComment_WithIntentAndDecisions(t *testing.T) {
	store := setupIntentTestDB(t)
	defer store.Close()

	run := baseRun("r-full")
	insertRun(t, store, run.ID)

	insertLayerEvent(t, store, run.ID, "intent.extracted", map[string]any{
		"first_user_msg": "Add password hashing",
		"summary":        "Implemented bcrypt for password storage",
		"spec_links": map[string]any{
			"jira_key":        "CLITEST-1",
			"confluence_urls": []string{"https://acme.atlassian.net/wiki/spaces/PROJ/pages/123"},
			"source_paths":    []string{},
		},
	})
	insertLayerEvent(t, store, run.ID, "decision.point", map[string]any{
		"chosen":    "bcrypt",
		"rejected":  []string{"argon2"},
		"rationale": "existing dependencies already pull bcrypt",
	})
	insertLayerEvent(t, store, run.ID, "decision.point", map[string]any{
		"chosen":    "single table",
		"rejected":  []string{"separate credentials table"},
		"rationale": "simpler schema",
	})

	comment := formatRunCommentWithStore(run, store)

	for _, want := range []string{
		"h3. Intent",
		"Add password hashing",
		"bcrypt for password storage",
		"CLITEST-1",
		"[Confluence|https://acme.atlassian.net/wiki/spaces/PROJ/pages/123]",
		"h3. Key Decisions",
		"*Chose:* bcrypt",
		"*Over:* argon2",
		"existing dependencies",
		"_(heuristic)_",
		"*Chose:* single table",
	} {
		if !strings.Contains(comment, want) {
			t.Errorf("missing %q in comment:\n%s", want, comment)
		}
	}
}

func TestFormatRunComment_WithConfluenceURLs(t *testing.T) {
	store := setupIntentTestDB(t)
	defer store.Close()

	run := baseRun("r-confluence")
	insertRun(t, store, run.ID)

	insertLayerEvent(t, store, run.ID, "intent.extracted", map[string]any{
		"first_user_msg": "Implement feature X",
		"summary":        "Done",
		"spec_links": map[string]any{
			"jira_key":        "",
			"confluence_urls": []string{"https://acme.atlassian.net/wiki/abc", "https://acme.atlassian.net/wiki/def"},
			"source_paths":    []string{},
		},
	})

	comment := formatRunCommentWithStore(run, store)

	if !strings.Contains(comment, "*Specs:*") {
		t.Errorf("missing Specs line:\n%s", comment)
	}
	if !strings.Contains(comment, "[Confluence|https://acme.atlassian.net/wiki/abc]") {
		t.Errorf("missing first confluence link:\n%s", comment)
	}
	if !strings.Contains(comment, "[Confluence|https://acme.atlassian.net/wiki/def]") {
		t.Errorf("missing second confluence link:\n%s", comment)
	}
}

func TestFormatRunComment_TruncatesLongFields(t *testing.T) {
	store := setupIntentTestDB(t)
	defer store.Close()

	run := baseRun("r-long")
	insertRun(t, store, run.ID)

	longMsg := strings.Repeat("A", 500)
	longSummary := strings.Repeat("B", 500)

	insertLayerEvent(t, store, run.ID, "intent.extracted", map[string]any{
		"first_user_msg": longMsg,
		"summary":        longSummary,
		"spec_links": map[string]any{
			"jira_key":        "",
			"confluence_urls": []string{},
			"source_paths":    []string{},
		},
	})

	comment := formatRunCommentWithStore(run, store)

	// Should contain truncation ellipsis.
	if !strings.Contains(comment, "…") {
		t.Errorf("expected truncation ellipsis for long fields:\n%s", comment)
	}
	// Must not contain the full 500-char string.
	if strings.Contains(comment, longMsg) {
		t.Errorf("first_user_msg should be truncated:\n%s", comment)
	}
}

func TestFormatRunComment_FailedRun_LegacyFormat(t *testing.T) {
	store := setupIntentTestDB(t)
	defer store.Close()

	run := baseRun("r-failed")
	run.Status = "failed"
	insertRun(t, store, run.ID)
	_, _ = store.Exec(`UPDATE runs SET status='failed' WHERE id=?`, run.ID)

	comment := formatRunCommentWithStore(run, store)

	if !strings.Contains(comment, "❌ *Agent Run Failed*") {
		t.Errorf("missing failed header:\n%s", comment)
	}
	if !strings.Contains(comment, "_Run failed - may need manual intervention._") {
		t.Errorf("missing failed footer:\n%s", comment)
	}
}

func TestFormatRunComment_DecisionWithNoRationale(t *testing.T) {
	store := setupIntentTestDB(t)
	defer store.Close()

	run := baseRun("r-no-rationale")
	insertRun(t, store, run.ID)

	insertLayerEvent(t, store, run.ID, "intent.extracted", map[string]any{
		"first_user_msg": "do something",
		"summary":        "done it",
		"spec_links":     map[string]any{"jira_key": "", "confluence_urls": []string{}, "source_paths": []string{}},
	})
	insertLayerEvent(t, store, run.ID, "decision.point", map[string]any{
		"chosen":    "approach A",
		"rejected":  []string{},
		"rationale": "",
	})

	comment := formatRunCommentWithStore(run, store)

	if !strings.Contains(comment, "_(heuristic)_") {
		t.Errorf("should still show heuristic label:\n%s", comment)
	}
}
