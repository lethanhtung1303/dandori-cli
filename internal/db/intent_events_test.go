package db

import (
	"testing"
	"time"
)

func seedRunForIntent(t *testing.T, d *LocalDB, runID string) {
	t.Helper()
	_, err := d.Exec(`
		INSERT INTO runs (id, jira_issue_key, agent_name, agent_type, user, workstation_id, started_at, status)
		VALUES (?, 'CLITEST-1', 'claude-code', 'claude_code', 'tester', 'ws-1', ?, 'done')
	`, runID, time.Now().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("seed run: %v", err)
	}
}

func TestGetIntentEvents_NoEvents(t *testing.T) {
	d := setupTestDB(t)
	defer d.Close()
	seedRunForIntent(t, d, "r1")

	result, err := d.GetIntentEvents("r1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Intent != nil {
		t.Errorf("expected nil Intent, got %+v", result.Intent)
	}
	if len(result.Decisions) != 0 {
		t.Errorf("expected 0 decisions, got %d", len(result.Decisions))
	}
}

func TestGetIntentEvents_IntentOnly(t *testing.T) {
	d := setupTestDB(t)
	defer d.Close()
	seedRunForIntent(t, d, "r2")

	insertEvent(t, d, "r2", "intent.extracted", map[string]any{
		"first_user_msg": "Fix the bug",
		"summary":        "Patched it",
		"spec_links": map[string]any{
			"jira_key":        "CLITEST-1",
			"confluence_urls": []any{"https://acme.atlassian.net/wiki/abc"},
			"source_paths":    []any{},
		},
	})

	result, err := d.GetIntentEvents("r2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Intent == nil {
		t.Fatal("expected Intent, got nil")
	}
	if result.Intent.FirstUserMsg != "Fix the bug" {
		t.Errorf("first_user_msg=%q", result.Intent.FirstUserMsg)
	}
	if result.Intent.Summary != "Patched it" {
		t.Errorf("summary=%q", result.Intent.Summary)
	}
	if result.Intent.SpecLinks.JiraKey != "CLITEST-1" {
		t.Errorf("jira_key=%q", result.Intent.SpecLinks.JiraKey)
	}
	if len(result.Intent.SpecLinks.ConfluenceURLs) != 1 {
		t.Errorf("confluence_urls len=%d, want 1", len(result.Intent.SpecLinks.ConfluenceURLs))
	}
	if len(result.Decisions) != 0 {
		t.Errorf("expected 0 decisions, got %d", len(result.Decisions))
	}
}

func TestGetIntentEvents_IntentWithDecisions(t *testing.T) {
	d := setupTestDB(t)
	defer d.Close()
	seedRunForIntent(t, d, "r3")

	insertEvent(t, d, "r3", "intent.extracted", map[string]any{
		"first_user_msg": "Add hashing",
		"summary":        "Used bcrypt",
		"spec_links":     map[string]any{"jira_key": "", "confluence_urls": []any{}, "source_paths": []any{}},
	})
	insertEvent(t, d, "r3", "decision.point", map[string]any{
		"chosen":    "bcrypt",
		"rejected":  []any{"argon2"},
		"rationale": "already in deps",
	})
	insertEvent(t, d, "r3", "decision.point", map[string]any{
		"chosen":    "SHA-256 for tokens",
		"rejected":  []any{},
		"rationale": "",
	})

	result, err := d.GetIntentEvents("r3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Intent == nil {
		t.Fatal("expected Intent")
	}
	if len(result.Decisions) != 2 {
		t.Errorf("expected 2 decisions, got %d", len(result.Decisions))
	}
	if result.Decisions[0].Chosen != "bcrypt" {
		t.Errorf("decision[0].chosen=%q, want bcrypt", result.Decisions[0].Chosen)
	}
	if len(result.Decisions[0].Rejected) != 1 || result.Decisions[0].Rejected[0] != "argon2" {
		t.Errorf("decision[0].rejected=%v, want [argon2]", result.Decisions[0].Rejected)
	}
}

func TestGetIntentEvents_DecisionsWithoutIntent(t *testing.T) {
	// Decision.point rows without intent.extracted → Decisions present, Intent nil.
	d := setupTestDB(t)
	defer d.Close()
	seedRunForIntent(t, d, "r4")

	insertEvent(t, d, "r4", "decision.point", map[string]any{
		"chosen":    "option A",
		"rejected":  []any{"option B"},
		"rationale": "faster",
	})

	result, err := d.GetIntentEvents("r4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Intent != nil {
		t.Errorf("expected nil Intent (no intent.extracted row)")
	}
	if len(result.Decisions) != 1 {
		t.Errorf("expected 1 decision, got %d", len(result.Decisions))
	}
}

func TestGetIntentEvents_MalformedEventSkipped(t *testing.T) {
	// A malformed JSON row should be silently skipped; other rows still parse.
	d := setupTestDB(t)
	defer d.Close()
	seedRunForIntent(t, d, "r5")

	// Insert a malformed intent.extracted row.
	_, err := d.Exec(`
		INSERT INTO events (run_id, layer, event_type, data, ts)
		VALUES (?, 4, 'intent.extracted', 'not-json', ?)
	`, "r5", "2026-01-01T00:00:00Z")
	if err != nil {
		t.Fatalf("insert malformed: %v", err)
	}

	// Insert a valid decision.point after the malformed row.
	insertEvent(t, d, "r5", "decision.point", map[string]any{
		"chosen":    "good choice",
		"rejected":  []any{},
		"rationale": "",
	})

	result, err := d.GetIntentEvents("r5")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Malformed intent row skipped → nil Intent.
	if result.Intent != nil {
		t.Errorf("malformed intent.extracted should be skipped")
	}
	// Valid decision still parsed.
	if len(result.Decisions) != 1 {
		t.Errorf("expected 1 decision, got %d", len(result.Decisions))
	}
}

func TestGetIntentEvents_WrongRunID(t *testing.T) {
	// Events for run "r-other" must not appear for run "r-mine".
	d := setupTestDB(t)
	defer d.Close()
	seedRunForIntent(t, d, "r-mine")
	seedRunForIntent(t, d, "r-other")

	insertEvent(t, d, "r-other", "intent.extracted", map[string]any{
		"first_user_msg": "other run msg",
		"summary":        "other",
		"spec_links":     map[string]any{"jira_key": "", "confluence_urls": []any{}, "source_paths": []any{}},
	})

	result, err := d.GetIntentEvents("r-mine")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Intent != nil {
		t.Errorf("should not see events from another run")
	}
}
