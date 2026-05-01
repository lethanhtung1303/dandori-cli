package server_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/phuc-nt/dandori-cli/internal/db"
)

// TestG9RunExpand_ReturnsIterationsAndIntentEvents seeds a run with a Jira
// issue key, 3 iteration events, and 2 layer-4 intent events; asserts the
// /api/g9/run/{id}/expand endpoint returns both arrays plus the issue_key.
func TestG9RunExpand_ReturnsIterationsAndIntentEvents(t *testing.T) {
	store := setupG9DB(t)
	defer store.Close()

	now := time.Now()
	runID := "run-exp-1"
	issueKey := "CLITEST-EXP"

	// Insert the run row.
	if _, err := store.Exec(`
		INSERT INTO runs (id, jira_issue_key, agent_name, agent_type, user, workstation_id,
		                  engineer_name, started_at, cost_usd, status)
		VALUES (?, ?, 'claude-code', 'claude_code', 'tester', 'ws-1', 'alice', ?, 1.5, 'done')
	`, runID, issueKey, now.UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	// Seed 3 iteration events (layer 1 — task.iteration.start).
	for i := 1; i <= 3; i++ {
		data := map[string]any{
			"round":           i,
			"issue_key":       issueKey,
			"transitioned_at": now.Add(time.Duration(i) * time.Minute).UTC().Format(time.RFC3339),
		}
		raw, _ := json.Marshal(data)
		if _, err := store.Exec(`
			INSERT INTO events (run_id, layer, event_type, data, ts)
			VALUES (?, 1, 'task.iteration.start', ?, ?)
		`, runID, string(raw), now.Add(time.Duration(i)*time.Minute).UTC().Format(time.RFC3339)); err != nil {
			t.Fatalf("seed iteration: %v", err)
		}
	}

	// Seed 2 layer-4 intent events.
	seedIntentEventG9(t, store, runID, "intent.extracted", map[string]any{
		"goal": "fix bug X", "constraints": []string{"keep tests green"},
	}, now)
	seedIntentEventG9(t, store, runID, "decision.made", map[string]any{
		"decision": "use approach A", "rationale": "simpler",
	}, now.Add(2*time.Minute))

	mux := newG9Mux(store)
	status, body := g9Get(t, mux, "/api/g9/run/"+runID+"/expand")
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", status, body)
	}

	var resp struct {
		RunID        string           `json:"run_id"`
		IssueKey     string           `json:"issue_key"`
		Iterations   []map[string]any `json:"iterations"`
		IntentEvents []map[string]any `json:"intent_events"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, body)
	}
	if resp.RunID != runID {
		t.Errorf("run_id=%q want %q", resp.RunID, runID)
	}
	if resp.IssueKey != issueKey {
		t.Errorf("issue_key=%q want %q", resp.IssueKey, issueKey)
	}
	if len(resp.Iterations) != 3 {
		t.Errorf("iterations len=%d want 3", len(resp.Iterations))
	}
	if len(resp.IntentEvents) != 2 {
		t.Errorf("intent_events len=%d want 2", len(resp.IntentEvents))
	}
}

// TestG9RunExpand_UnknownRunID_Returns404 ensures we 404 cleanly.
func TestG9RunExpand_UnknownRunID_Returns404(t *testing.T) {
	store := setupG9DB(t)
	defer store.Close()

	mux := newG9Mux(store)
	status, body := g9Get(t, mux, "/api/g9/run/nonexistent/expand")
	if status != http.StatusNotFound {
		t.Errorf("status=%d want 404; body=%s", status, body)
	}
}

// TestG9RunExpand_RunWithoutEvents_ReturnsEmptyArrays asserts that a run with
// no iterations or intent events returns 200 with empty arrays (not null).
func TestG9RunExpand_RunWithoutEvents_ReturnsEmptyArrays(t *testing.T) {
	store := setupG9DB(t)
	defer store.Close()

	now := time.Now()
	runID := "run-exp-empty"
	if _, err := store.Exec(`
		INSERT INTO runs (id, jira_issue_key, agent_name, agent_type, user, workstation_id,
		                  engineer_name, started_at, cost_usd, status)
		VALUES (?, 'CLITEST-EMPTY', 'claude-code', 'claude_code', 'tester', 'ws-1', 'bob', ?, 0.1, 'done')
	`, runID, now.UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	mux := newG9Mux(store)
	status, body := g9Get(t, mux, "/api/g9/run/"+runID+"/expand")
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", status, body)
	}
	// Body must contain literal "iterations":[] and "intent_events":[] (not null).
	bodyStr := string(body)
	if !contains(bodyStr, `"iterations":[]`) {
		t.Errorf("expected empty iterations array, body=%s", bodyStr)
	}
	if !contains(bodyStr, `"intent_events":[]`) {
		t.Errorf("expected empty intent_events array, body=%s", bodyStr)
	}
}

// contains is a small helper to avoid importing strings just for this check.
func contains(haystack, needle string) bool {
	if len(needle) == 0 {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

// Compile-time silence unused import if any.
var _ = db.LocalDB{}
