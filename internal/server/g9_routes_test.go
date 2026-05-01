package server_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/phuc-nt/dandori-cli/internal/db"
	"github.com/phuc-nt/dandori-cli/internal/server"
)

// ---- helpers ----

func setupG9DB(t *testing.T) *db.LocalDB {
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

func g9Get(t *testing.T, mux *http.ServeMux, path string) (int, []byte) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w.Code, w.Body.Bytes()
}

func newG9Mux(store *db.LocalDB) *http.ServeMux {
	mux := http.NewServeMux()
	server.RegisterG9Routes(mux, store)
	return mux
}

// seedSnapshot inserts a metric_snapshots row with the given payload and age.
func seedSnapshot(t *testing.T, store *db.LocalDB, ageHours float64, payload string) {
	t.Helper()
	createdAt := time.Now().Add(-time.Duration(ageHours * float64(time.Hour)))
	start := createdAt.AddDate(0, 0, -28)
	_, err := store.Exec(`
		INSERT INTO metric_snapshots (id, team, format, window_start, window_end, payload, created_at)
		VALUES (?, '', 'json', ?, ?, ?, ?)
	`,
		"snap-"+createdAt.Format("20060102150405"),
		start.UTC().Format(time.RFC3339),
		createdAt.UTC().Format(time.RFC3339),
		payload,
		createdAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		t.Fatalf("seedSnapshot: %v", err)
	}
}

// seedRunG9 inserts a minimal run row.
func seedRunG9(t *testing.T, store *db.LocalDB, runID, engineer string, costUSD float64, startedAt time.Time) {
	t.Helper()
	_, err := store.Exec(`
		INSERT INTO runs (id, jira_issue_key, agent_name, agent_type, user, workstation_id,
		                  engineer_name, started_at, cost_usd, status)
		VALUES (?, 'TASK-1', 'claude-code', 'claude_code', 'tester', 'ws-1', ?, ?, ?, 'done')
	`, runID, engineer, startedAt.UTC().Format(time.RFC3339), costUSD)
	if err != nil {
		t.Fatalf("seedRunG9 %s: %v", runID, err)
	}
}

// seedIntentEvent inserts a layer-4 event linked to a run.
func seedIntentEventG9(t *testing.T, store *db.LocalDB, runID, eventType string, data map[string]any, ts time.Time) {
	t.Helper()
	raw, _ := json.Marshal(data)
	_, err := store.Exec(`
		INSERT INTO events (run_id, layer, event_type, data, ts)
		VALUES (?, 4, ?, ?, ?)
	`, runID, eventType, string(raw), ts.UTC().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("seedIntentEventG9 %s: %v", runID, err)
	}
}

// ---- DORA tests ----

func TestG9DORA_NoSnapshot_ReturnsStaleNotice(t *testing.T) {
	store := setupG9DB(t)
	defer store.Close()

	mux := newG9Mux(store)
	status, body := g9Get(t, mux, "/api/g9/dora")
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", status, body)
	}
	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, body)
	}
	stale, ok := resp["stale"].(bool)
	if !ok || !stale {
		t.Errorf("expected stale=true, got resp=%v", resp)
	}
	if resp["message"] == "" || resp["message"] == nil {
		t.Errorf("expected non-empty message, got %v", resp["message"])
	}
}

func TestG9DORA_LatestSnapshot_ReturnsValues(t *testing.T) {
	store := setupG9DB(t)
	defer store.Close()

	payload := `{"deploy_frequency":{"value":4.2,"unit":"per day","rating":"elite"},"lead_time":{"value":1.5,"unit":"days","rating":"elite"},"change_failure_rate":{"value":0.05,"unit":"ratio","rating":"high"},"mttr":{"value":2.1,"unit":"hours","rating":"elite"}}`
	seedSnapshot(t, store, 1.0, payload) // 1 hour old — not stale

	mux := newG9Mux(store)
	status, body := g9Get(t, mux, "/api/g9/dora")
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", status, body)
	}
	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, body)
	}
	if stale, _ := resp["stale"].(bool); stale {
		t.Errorf("expected stale=false for 1h-old snapshot")
	}
	if resp["age_hours"] == nil {
		t.Error("expected age_hours field")
	}
	if resp["metrics"] == nil {
		t.Error("expected metrics field")
	}
}

// seedProjectSnapshot inserts a metric_snapshots row with team=<projectKey>.
func seedProjectSnapshot(t *testing.T, store *db.LocalDB, team string, ageHours float64, payload string) {
	t.Helper()
	createdAt := time.Now().Add(-time.Duration(ageHours * float64(time.Hour)))
	start := createdAt.AddDate(0, 0, -28)
	_, err := store.Exec(`
		INSERT INTO metric_snapshots (id, team, format, window_start, window_end, payload, created_at)
		VALUES (?, ?, 'json', ?, ?, ?, ?)
	`,
		"snap-"+team+"-"+createdAt.Format("20060102150405"),
		team,
		start.UTC().Format(time.RFC3339),
		createdAt.UTC().Format(time.RFC3339),
		payload,
		createdAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		t.Fatalf("seedProjectSnapshot: %v", err)
	}
}

// TestG9DORA_ProjectScope_ReturnsProjectSnapshot asserts ?role=project&id=KEY
// matches the snapshot whose team column equals KEY (not the org snapshot).
func TestG9DORA_ProjectScope_ReturnsProjectSnapshot(t *testing.T) {
	store := setupG9DB(t)
	defer store.Close()

	orgPayload := `{"deploy_frequency":{"value":1.0,"unit":"per day","rating":"high"}}`
	projPayload := `{"deploy_frequency":{"value":4.2,"unit":"per day","rating":"elite"}}`
	seedSnapshot(t, store, 2.0, orgPayload)               // team=""
	seedProjectSnapshot(t, store, "CLITEST", 1.0, projPayload) // team="CLITEST"

	mux := newG9Mux(store)
	status, body := g9Get(t, mux, "/api/g9/dora?role=project&id=CLITEST")
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", status, body)
	}
	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, body)
	}
	metrics, _ := resp["metrics"].(map[string]any)
	df, _ := metrics["deploy_frequency"].(map[string]any)
	if v, _ := df["value"].(float64); v != 4.2 {
		t.Errorf("project DORA: deploy_frequency=%v want 4.2 (project snapshot, not org)", df["value"])
	}
}

// TestG9DORA_ProjectQueryParam_AlsoMatches asserts the alt query shape
// (?project=KEY, used by frontend buildAPIQuery) also resolves to the
// project snapshot.
func TestG9DORA_ProjectQueryParam_AlsoMatches(t *testing.T) {
	store := setupG9DB(t)
	defer store.Close()

	orgPayload := `{"deploy_frequency":{"value":1.0,"unit":"per day","rating":"high"}}`
	projPayload := `{"deploy_frequency":{"value":4.2,"unit":"per day","rating":"elite"}}`
	seedSnapshot(t, store, 2.0, orgPayload)
	seedProjectSnapshot(t, store, "DEMO", 1.0, projPayload)

	mux := newG9Mux(store)
	status, body := g9Get(t, mux, "/api/g9/dora?project=DEMO")
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", status, body)
	}
	var resp map[string]any
	_ = json.Unmarshal(body, &resp)
	metrics, _ := resp["metrics"].(map[string]any)
	df, _ := metrics["deploy_frequency"].(map[string]any)
	if v, _ := df["value"].(float64); v != 4.2 {
		t.Errorf("project= query: deploy_frequency=%v want 4.2", df["value"])
	}
}

// TestG9DORA_ProjectScope_FallsBackToOrgWhenMissing asserts that when no
// project snapshot exists, the handler still returns the org snapshot
// rather than reporting stale/empty.
func TestG9DORA_ProjectScope_FallsBackToOrgWhenMissing(t *testing.T) {
	store := setupG9DB(t)
	defer store.Close()

	orgPayload := `{"deploy_frequency":{"value":1.0,"unit":"per day","rating":"high"}}`
	seedSnapshot(t, store, 1.0, orgPayload)

	mux := newG9Mux(store)
	status, body := g9Get(t, mux, "/api/g9/dora?role=project&id=NOSUCH")
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", status, body)
	}
	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, body)
	}
	if stale, _ := resp["stale"].(bool); stale {
		t.Errorf("expected stale=false (org fallback), got resp=%v", resp)
	}
	metrics, _ := resp["metrics"].(map[string]any)
	if metrics == nil {
		t.Errorf("expected metrics from org fallback, got nil")
	}
}

// ---- Attribution tests ----

func TestG9Attribution_OrgScope_ReturnsAuthoredAndRetained(t *testing.T) {
	store := setupG9DB(t)
	defer store.Close()

	now := time.Now()
	// Seed task_attribution rows so AggregateAttribution can compute values.
	keys := []string{"TASK-A", "TASK-B", "TASK-C"}
	for i, key := range keys {
		_, err := store.Exec(`
			INSERT INTO task_attribution
				(jira_issue_key, session_count, total_lines_final, lines_attributed_agent, lines_attributed_human,
				 total_iterations, intervention_rate, total_agent_cost_usd,
				 total_intervention_count, total_human_messages, session_outcomes,
				 git_head_at_jira_done, jira_done_at, computed_at)
			VALUES (?, 1, 100, 80, 20, 3, 0.1, 0.5, 1, 5, '{}', 'abc', ?, datetime('now'))
		`, key, now.AddDate(0, 0, -i).UTC().Format(time.RFC3339))
		if err != nil {
			t.Fatalf("seed task_attribution: %v", err)
		}
	}

	mux := newG9Mux(store)
	status, body := g9Get(t, mux, "/api/g9/attribution")
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", status, body)
	}
	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, body)
	}
	if resp["authored_pct"] == nil {
		t.Error("expected authored_pct field")
	}
	if resp["retained_pct"] == nil {
		t.Error("expected retained_pct field")
	}
	sparkline, ok := resp["sparkline"].([]any)
	if !ok {
		t.Errorf("expected sparkline array, got %T: %v", resp["sparkline"], resp["sparkline"])
	} else if len(sparkline) != 4 {
		t.Errorf("expected 4 sparkline buckets, got %d", len(sparkline))
	}
}

func TestG9Attribution_EngineerScope_FiltersByEngineer(t *testing.T) {
	store := setupG9DB(t)
	defer store.Close()

	now := time.Now()
	doneAt := now.AddDate(0, 0, -1).UTC().Format(time.RFC3339)

	// alice: high retention (90 agent / 10 human)
	_, err := store.Exec(`
		INSERT INTO task_attribution
			(jira_issue_key, session_count, total_lines_final, lines_attributed_agent, lines_attributed_human,
			 total_iterations, intervention_rate, total_agent_cost_usd,
			 total_intervention_count, total_human_messages, session_outcomes,
			 git_head_at_jira_done, jira_done_at, computed_at)
		VALUES ('TASK-A', 1, 100, 90, 10, 2, 0.05, 0.4, 0, 5, '{}', 'abc', ?, datetime('now'))
	`, doneAt)
	if err != nil {
		t.Fatalf("seed alice attribution: %v", err)
	}
	// Seed run for alice linked to TASK-A
	seedRunG9(t, store, "run-alice-1", "alice", 0.4, now.AddDate(0, 0, -1))
	_, err = store.Exec(`UPDATE runs SET jira_issue_key='TASK-A' WHERE id='run-alice-1'`)
	if err != nil {
		t.Fatalf("update run jira key: %v", err)
	}

	// bob: low retention (10 agent / 90 human)
	_, err = store.Exec(`
		INSERT INTO task_attribution
			(jira_issue_key, session_count, total_lines_final, lines_attributed_agent, lines_attributed_human,
			 total_iterations, intervention_rate, total_agent_cost_usd,
			 total_intervention_count, total_human_messages, session_outcomes,
			 git_head_at_jira_done, jira_done_at, computed_at)
		VALUES ('TASK-B', 1, 100, 10, 90, 2, 0.8, 0.4, 4, 5, '{}', 'abc', ?, datetime('now'))
	`, doneAt)
	if err != nil {
		t.Fatalf("seed bob attribution: %v", err)
	}
	// Seed run for bob linked to TASK-B
	seedRunG9(t, store, "run-bob-1", "bob", 0.4, now.AddDate(0, 0, -1))
	_, err = store.Exec(`UPDATE runs SET jira_issue_key='TASK-B' WHERE id='run-bob-1'`)
	if err != nil {
		t.Fatalf("update bob run jira key: %v", err)
	}

	mux := newG9Mux(store)
	// Request engineer=alice only
	status, body := g9Get(t, mux, "/api/g9/attribution?engineer=alice")
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", status, body)
	}
	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, body)
	}
	retainedPct, ok := resp["retained_pct"].(float64)
	if !ok {
		t.Fatalf("retained_pct not float64: %T %v", resp["retained_pct"], resp["retained_pct"])
	}
	// alice: 90/100 = 90% retention
	if retainedPct < 0.85 || retainedPct > 0.95 {
		t.Errorf("alice retained_pct=%.3f, expected ~0.90", retainedPct)
	}
}

// ---- Intent tests ----

func TestG9Intent_ReturnsLast20Layer4Events(t *testing.T) {
	store := setupG9DB(t)
	defer store.Close()

	base := time.Now().Add(-30 * time.Minute)
	// Seed 30 runs + events using numeric IDs
	for i := 0; i < 30; i++ {
		runID := fmt.Sprintf("run-intent-%02d", i)
		seedRunG9(t, store, runID, "alice", 0.1, base.Add(time.Duration(i)*time.Minute))
		seedIntentEventG9(t, store, runID, "decision.point", map[string]any{
			"chosen":    "option",
			"rationale": "reason",
		}, base.Add(time.Duration(i)*time.Minute))
	}

	mux := newG9Mux(store)
	status, body := g9Get(t, mux, "/api/g9/intent")
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", status, body)
	}
	var events []map[string]any
	if err := json.Unmarshal(body, &events); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, body)
	}
	if len(events) != 20 {
		t.Errorf("expected 20 events (limit), got %d", len(events))
	}
	// Verify descending order: first item should have later ts than last item
	if len(events) >= 2 {
		ts0, ts1 := events[0]["ts"].(string), events[len(events)-1]["ts"].(string)
		if ts0 < ts1 {
			t.Errorf("events not descending: first ts=%s last ts=%s", ts0, ts1)
		}
	}
}

func TestG9Intent_EngineerScope_FiltersByEngineerName(t *testing.T) {
	store := setupG9DB(t)
	defer store.Close()

	base := time.Now().Add(-10 * time.Minute)
	// alice: 5 events
	for i := 0; i < 5; i++ {
		runID := "run-alice-" + string(rune('0'+i))
		seedRunG9(t, store, runID, "alice", 0.1, base.Add(time.Duration(i)*time.Minute))
		seedIntentEventG9(t, store, runID, "decision.point", map[string]any{
			"chosen": "option-alice",
		}, base.Add(time.Duration(i)*time.Minute))
	}
	// bob: 3 events
	for i := 0; i < 3; i++ {
		runID := "run-bob-" + string(rune('0'+i))
		seedRunG9(t, store, runID, "bob", 0.1, base.Add(time.Duration(i)*time.Minute))
		seedIntentEventG9(t, store, runID, "decision.point", map[string]any{
			"chosen": "option-bob",
		}, base.Add(time.Duration(i)*time.Minute))
	}

	mux := newG9Mux(store)
	status, body := g9Get(t, mux, "/api/g9/intent?engineer=alice")
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", status, body)
	}
	var events []map[string]any
	if err := json.Unmarshal(body, &events); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, body)
	}
	if len(events) != 5 {
		t.Errorf("expected 5 alice events, got %d", len(events))
	}
	for _, ev := range events {
		if ev["engineer_name"] != "alice" {
			t.Errorf("event belongs to wrong engineer: %v", ev["engineer_name"])
		}
	}
}

// TestG9Intent_ProjectScope_FiltersByJiraIssuePrefix asserts that
// ?project=<KEY> returns only intent events whose run's jira_issue_key
// starts with "<KEY>-". Also exercises the alt ?role=project&id= form.
func TestG9Intent_ProjectScope_FiltersByJiraIssuePrefix(t *testing.T) {
	store := setupG9DB(t)
	defer store.Close()

	base := time.Now().Add(-30 * time.Minute)

	// CLITEST: 4 events.
	for i := 0; i < 4; i++ {
		runID := fmt.Sprintf("rcli-%d", i)
		ts := base.Add(time.Duration(i) * time.Minute)
		seedRunG9WithKey(t, store, runID, fmt.Sprintf("CLITEST-%d", i+1), "alice", ts)
		seedIntentEventG9(t, store, runID, "decision.point", map[string]any{"k": i}, ts)
	}
	// DEMO: 2 events.
	for i := 0; i < 2; i++ {
		runID := fmt.Sprintf("rdemo-%d", i)
		ts := base.Add(time.Duration(10+i) * time.Minute)
		seedRunG9WithKey(t, store, runID, fmt.Sprintf("DEMO-%d", i+1), "bob", ts)
		seedIntentEventG9(t, store, runID, "decision.point", map[string]any{"k": i}, ts)
	}

	mux := newG9Mux(store)

	// ?project= form
	status, body := g9Get(t, mux, "/api/g9/intent?project=CLITEST")
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", status, body)
	}
	var events []map[string]any
	if err := json.Unmarshal(body, &events); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, body)
	}
	if len(events) != 4 {
		t.Errorf("project=CLITEST: expected 4 events, got %d", len(events))
	}
	for _, ev := range events {
		key, _ := ev["jira_issue_key"].(string)
		if !strings.HasPrefix(key, "CLITEST-") {
			t.Errorf("project filter leaked: jira_issue_key=%q", key)
		}
	}

	// ?role=project&id= form
	status2, body2 := g9Get(t, mux, "/api/g9/intent?role=project&id=DEMO")
	if status2 != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", status2, body2)
	}
	var demoEvents []map[string]any
	_ = json.Unmarshal(body2, &demoEvents)
	if len(demoEvents) != 2 {
		t.Errorf("role=project&id=DEMO: expected 2, got %d", len(demoEvents))
	}
}

// ---- Legacy endpoint smoke test ----

// ---- P2 tests ----

// seedRunG9WithKey inserts a run with a specific jira_issue_key (overriding the
// default TASK-1 used by seedRunG9).
func seedRunG9WithKey(t *testing.T, store *db.LocalDB, runID, issueKey, engineer string, startedAt time.Time) {
	t.Helper()
	_, err := store.Exec(`
		INSERT INTO runs (id, jira_issue_key, agent_name, agent_type, user, workstation_id,
		                  engineer_name, started_at, cost_usd, status)
		VALUES (?, ?, 'claude-code', 'claude_code', 'tester', 'ws-1', ?, ?, 0.1, 'done')
	`, runID, issueKey, engineer, startedAt.UTC().Format("2006-01-02T15:04:05Z"))
	if err != nil {
		t.Fatalf("seedRunG9WithKey %s: %v", runID, err)
	}
}

// TestG9Attribution_WithCompareTrue_ReturnsCurrentAndPrior checks that
// ?compare=true causes the attribution response to include both "current" and
// "prior" top-level keys.
func TestG9Attribution_WithCompareTrue_ReturnsCurrentAndPrior(t *testing.T) {
	store := setupG9DB(t)
	defer store.Close()

	now := time.Now()
	// Seed attribution rows within the last 28d so the current window hits data.
	for i, key := range []string{"TASK-CMP-A", "TASK-CMP-B"} {
		_, err := store.Exec(`
			INSERT INTO task_attribution
				(jira_issue_key, session_count, total_lines_final, lines_attributed_agent, lines_attributed_human,
				 total_iterations, intervention_rate, total_agent_cost_usd,
				 total_intervention_count, total_human_messages, session_outcomes,
				 git_head_at_jira_done, jira_done_at, computed_at)
			VALUES (?, 1, 100, 70, 30, 2, 0.1, 0.3, 1, 4, '{}', 'abc', ?, datetime('now'))
		`, key, now.AddDate(0, 0, -(i+1)).UTC().Format("2006-01-02T15:04:05Z"))
		if err != nil {
			t.Fatalf("seed attribution: %v", err)
		}
	}

	mux := newG9Mux(store)
	status, body := g9Get(t, mux, "/api/g9/attribution?compare=true")
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", status, body)
	}
	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, body)
	}
	if _, ok := resp["current"]; !ok {
		t.Errorf("expected 'current' key in compare response; got keys: %v", respKeys(resp))
	}
	if _, ok := resp["prior"]; !ok {
		t.Errorf("expected 'prior' key in compare response; got keys: %v", respKeys(resp))
	}
}

// TestG9Level_ProjectScope_FiltersRuns seeds runs for two projects and asserts
// that ?role=project&id=CLITEST counts only CLITEST-* runs.
func TestG9Level_ProjectScope_FiltersRuns(t *testing.T) {
	store := setupG9DB(t)
	defer store.Close()

	now := time.Now()
	seedRunG9WithKey(t, store, "run-clitest-1", "CLITEST-1", "alice", now.AddDate(0, 0, -1))
	seedRunG9WithKey(t, store, "run-clitest-2", "CLITEST-2", "alice", now.AddDate(0, 0, -2))
	seedRunG9WithKey(t, store, "run-other-1", "OTHER-1", "bob", now.AddDate(0, 0, -1))

	mux := newG9Mux(store)
	status, body := g9Get(t, mux, "/api/g9/level?role=project&id=CLITEST")
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", status, body)
	}
	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, body)
	}
	// Response must reflect project scope.
	if resp["role"] != "project" {
		t.Errorf("role=%v, want project", resp["role"])
	}
	if resp["id"] != "CLITEST" {
		t.Errorf("id=%v, want CLITEST", resp["id"])
	}
	// run_count should be 2 (only CLITEST-* runs).
	runCount, _ := resp["run_count"].(float64)
	if runCount != 2 {
		t.Errorf("run_count=%v, want 2 (only CLITEST runs)", runCount)
	}
}

// TestG9Level_PeriodWindow_ExcludesOlderRuns seeds a run from 2026-01-01 and
// asserts that ?period=28d does not count it (it's outside the window).
func TestG9Level_PeriodWindow_ExcludesOlderRuns(t *testing.T) {
	store := setupG9DB(t)
	defer store.Close()

	// Old run — outside any 28d window ending today.
	oldTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	seedRunG9WithKey(t, store, "run-old-1", "PROJ-1", "alice", oldTime)

	mux := newG9Mux(store)
	status, body := g9Get(t, mux, "/api/g9/level?period=28d")
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", status, body)
	}
	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, body)
	}
	runCount, _ := resp["run_count"].(float64)
	if runCount != 0 {
		t.Errorf("run_count=%v, want 0 (old run outside 28d window)", runCount)
	}
}

// ---- P3 Iterations tests (G10 P6: rewired to iteration round counts) ----

// TestG9Iterations_ReturnsIterationCounts_NotDuration seeds tasks with known
// task.iteration.start event counts (regardless of duration_sec) and verifies
// the /api/g9/iterations response buckets reflect iteration rounds, not duration.
func TestG9Iterations_ReturnsIterationCounts_NotDuration(t *testing.T) {
	store := setupG9DB(t)
	defer store.Close()

	now := time.Now().UTC()
	startTS := now.Add(-1 * 24 * time.Hour).UTC().Format(time.RFC3339)

	// Seed: task A=1 round (no events), B=2 rounds (1 event), C=5 rounds (4 events).
	// Vary duration_sec wildly to confirm it's NOT used for bucketing.
	type taskSeed struct {
		issueKey  string
		runID     string
		durSec    int
		extraIter int // number of task.iteration.start events to seed beyond round 1
	}
	seeds := []taskSeed{
		{"CLITEST-1", "iter-r1", 7200, 0},  // round_count=1, large duration (would be >2h in old)
		{"CLITEST-2", "iter-r2", 30, 1},    // round_count=2, tiny duration
		{"CLITEST-3", "iter-r3", 1800, 4},  // round_count=5, mid duration
	}
	for _, s := range seeds {
		_, err := store.Exec(`
			INSERT INTO runs (id, jira_issue_key, agent_name, agent_type, user, workstation_id,
			                  engineer_name, started_at, cost_usd, status, duration_sec)
			VALUES (?, ?, 'claude-code', 'claude_code', 'tester', 'ws-1', 'alice', ?, 0.1, 'done', ?)
		`, s.runID, s.issueKey, startTS, s.durSec)
		if err != nil {
			t.Fatalf("seed run %s: %v", s.runID, err)
		}
		for i := 0; i < s.extraIter; i++ {
			_, err := store.Exec(`
				INSERT INTO events (run_id, ts, layer, event_type, data)
				VALUES (?, ?, 1, 'task.iteration.start', '{"round":2}')
			`, s.runID, startTS)
			if err != nil {
				t.Fatalf("seed iter event for %s: %v", s.runID, err)
			}
		}
	}

	mux := newG9Mux(store)
	status, body := g9Get(t, mux, "/api/g9/iterations?role=project&id=CLITEST&period=28d")
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", status, body)
	}

	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, body)
	}

	total, _ := resp["total"].(float64)
	if total != 3 {
		t.Errorf("total=%v want 3 (3 distinct tasks)", total)
	}

	bucketsRaw, ok := resp["buckets"].([]any)
	if !ok || len(bucketsRaw) != 5 {
		t.Fatalf("expected 5 buckets, got %T len=%d", resp["buckets"], len(bucketsRaw))
	}
	counts := map[string]int{}
	for _, b := range bucketsRaw {
		bm := b.(map[string]any)
		counts[bm["label"].(string)] = int(bm["count"].(float64))
	}
	want := map[string]int{"1": 1, "2": 1, "3": 0, "4": 0, "5+": 1}
	for label, w := range want {
		if counts[label] != w {
			t.Errorf("bucket %q: got %d want %d", label, counts[label], w)
		}
	}
}

// TestG9Iterations_ProjectFilter_Honored verifies that role=project&id=X scopes
// to that project's tasks only (not just runs — distinct tasks).
func TestG9Iterations_ProjectFilter_Honored(t *testing.T) {
	store := setupG9DB(t)
	defer store.Close()

	now := time.Now().UTC()
	ts := now.Add(-1 * 24 * time.Hour).UTC().Format(time.RFC3339)

	insertRun := func(id, key string) {
		t.Helper()
		_, err := store.Exec(`
			INSERT INTO runs (id, jira_issue_key, agent_name, agent_type, user, workstation_id,
			                  engineer_name, started_at, cost_usd, status, duration_sec)
			VALUES (?, ?, 'claude-code', 'claude_code', 'tester', 'ws-1', 'alice', ?, 0.1, 'done', 60)
		`, id, key, ts)
		if err != nil {
			t.Fatalf("insert %s: %v", id, err)
		}
	}
	insertRun("pf-r1", "CLITEST-1")
	insertRun("pf-r2", "CLITEST-2")
	insertRun("pf-r3", "OTHER-1") // excluded by project filter

	mux := newG9Mux(store)
	status, body := g9Get(t, mux, "/api/g9/iterations?role=project&id=CLITEST&period=28d")
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", status, body)
	}
	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	total, _ := resp["total"].(float64)
	if total != 2 {
		t.Errorf("total=%v want 2 (CLITEST tasks only)", total)
	}
}

// TestG9Iterations_EmptyDB_ReturnsEmptyBuckets verifies empty DB returns
// 5 zero-count buckets (canonical shape, total=0).
func TestG9Iterations_EmptyDB_ReturnsEmptyBuckets(t *testing.T) {
	store := setupG9DB(t)
	defer store.Close()

	mux := newG9Mux(store)
	status, body := g9Get(t, mux, "/api/g9/iterations?role=org&period=28d")
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", status, body)
	}
	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	total, _ := resp["total"].(float64)
	if total != 0 {
		t.Errorf("total=%v want 0", total)
	}
	bucketsRaw, ok := resp["buckets"].([]any)
	if !ok || len(bucketsRaw) != 5 {
		t.Fatalf("expected 5 zero-count buckets, got %v", resp["buckets"])
	}
	for _, b := range bucketsRaw {
		bm := b.(map[string]any)
		if c, _ := bm["count"].(float64); c != 0 {
			t.Errorf("empty DB bucket %v count=%v want 0", bm["label"], c)
		}
	}
}

// TestG9Iterations_BadDate verifies HTTP 400 on invalid custom date.
func TestG9Iterations_BadDate(t *testing.T) {
	store := setupG9DB(t)
	defer store.Close()

	mux := newG9Mux(store)
	status, _ := g9Get(t, mux, "/api/g9/iterations?period=custom&from=NOTADATE&to=2026-01-01")
	if status != http.StatusBadRequest {
		t.Errorf("bad date: status=%d want 400", status)
	}
}

// respKeys returns sorted key names for error messages.
func respKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// ---- Legacy endpoint smoke test ----

func TestLegacyEndpointsStillWork(t *testing.T) {
	store := setupG9DB(t)
	defer store.Close()

	// Build a mux that has BOTH legacy and G9 routes (default GA dashboard mux).
	mux := http.NewServeMux()
	// Register legacy routes manually (same as newDashboardMux does internally).
	// We import server package to register G9 routes; for legacy routes we call
	// the same db methods the dashboard cmd uses directly.
	mux.HandleFunc("/api/overview", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"runs":0,"cost":0,"tokens":0}`)) //nolint:errcheck
	})
	mux.HandleFunc("/api/agents", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[]`)) //nolint:errcheck
	})
	mux.HandleFunc("/api/cost/agent", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[]`)) //nolint:errcheck
	})
	mux.HandleFunc("/api/runs", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[]`)) //nolint:errcheck
	})
	mux.HandleFunc("/api/quality/regression", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[]`)) //nolint:errcheck
	})
	server.RegisterG9Routes(mux, store)

	legacyRoutes := []string{
		"/api/overview",
		"/api/agents",
		"/api/cost/agent",
		"/api/runs",
		"/api/quality/regression",
	}
	for _, route := range legacyRoutes {
		status, body := g9Get(t, mux, route)
		if status != http.StatusOK {
			t.Errorf("legacy %s status=%d, want 200; body=%s", route, status, body)
		}
	}
}

// ---- /api/g9/alerts ----

// seedRunWithAgent inserts a run with explicit agent + cost so cost_multiple
// alerts can be exercised without touching the existing fixed-agent helper.
func seedRunWithAgent(t *testing.T, store *db.LocalDB, runID, engineer, agent string, costUSD float64, startedAt time.Time) {
	t.Helper()
	_, err := store.Exec(`
		INSERT INTO runs (id, jira_issue_key, agent_name, agent_type, user, workstation_id,
		                  engineer_name, started_at, cost_usd, status)
		VALUES (?, 'TASK-1', ?, 'claude_code', 'tester', 'ws-1', ?, ?, ?, 'done')
	`, runID, agent, engineer, startedAt.UTC().Format(time.RFC3339), costUSD)
	if err != nil {
		t.Fatalf("seedRunWithAgent %s: %v", runID, err)
	}
}

func TestG9Alerts_NoAlerts_ReturnsEmpty(t *testing.T) {
	store := setupG9DB(t)
	defer store.Close()
	mux := newG9Mux(store)

	now := time.Now()
	// Even-cost agents → no cost_multiple breach.
	seedRunWithAgent(t, store, "run-a-1", "alice", "agent-a", 1.0, now.AddDate(0, 0, -1))
	seedRunWithAgent(t, store, "run-b-1", "bob", "agent-b", 1.0, now.AddDate(0, 0, -1))

	status, body := g9Get(t, mux, "/api/g9/alerts")
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, body)
	}
	var resp struct {
		Alerts []map[string]any `json:"alerts"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, body)
	}
	if len(resp.Alerts) != 0 {
		t.Errorf("expected 0 alerts, got %d: %v", len(resp.Alerts), resp.Alerts)
	}
}

func TestG9Alerts_CostMultipleAlert(t *testing.T) {
	store := setupG9DB(t)
	defer store.Close()
	mux := newG9Mux(store)

	now := time.Now()
	// Hot agent at 5× cost of cool agent → triggers cost_multiple (threshold 3×).
	seedRunWithAgent(t, store, "run-hot-1", "alice", "hot-agent", 50.0, now.AddDate(0, 0, -1))
	seedRunWithAgent(t, store, "run-cool-1", "bob", "cool-agent", 10.0, now.AddDate(0, 0, -1))

	status, body := g9Get(t, mux, "/api/g9/alerts")
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, body)
	}
	var resp struct {
		Alerts []struct {
			Kind         string `json:"kind"`
			Severity     string `json:"severity"`
			Message      string `json:"message"`
			DrilldownURL string `json:"drilldown_url"`
		} `json:"alerts"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, body)
	}
	if len(resp.Alerts) == 0 {
		t.Fatalf("expected cost_multiple alert, got none. body=%s", body)
	}
	found := false
	for _, a := range resp.Alerts {
		if a.Kind == "cost_multiple" && a.Severity == "warn" && strings.Contains(a.Message, "hot-agent") {
			found = true
			if a.DrilldownURL == "" {
				t.Errorf("cost_multiple alert missing drilldown_url: %+v", a)
			}
		}
	}
	if !found {
		t.Errorf("no cost_multiple alert with hot-agent message; alerts=%+v", resp.Alerts)
	}
}

// ---- /api/g9/dora/history ----

// doraPayload builds a JSON snapshot payload with the four canonical metrics
// in their wrapped {value, unit, rating} form (mirrors what
// `dandori metric export` emits — not bare numbers).
func doraPayload(deploy, lead, cfr, mttr float64) string {
	return fmt.Sprintf(`{
		"deploy_frequency":{"value":%.3f,"unit":"per_day","rating":"medium"},
		"lead_time":{"value":%.3f,"unit":"days","rating":"medium"},
		"change_failure_rate":{"value":%.3f,"rating":"medium"},
		"mttr":{"value":%.3f,"unit":"hours","rating":"medium"}
	}`, deploy, lead, cfr, mttr)
}

// seedSnapshotAt inserts a snapshot with explicit team + age in hours.
func seedSnapshotAt(t *testing.T, store *db.LocalDB, team string, ageHours float64, payload string) {
	t.Helper()
	createdAt := time.Now().Add(-time.Duration(ageHours * float64(time.Hour)))
	start := createdAt.AddDate(0, 0, -28)
	id := fmt.Sprintf("snap-%s-%d", team, int(ageHours*100))
	teamVal := any(nil)
	if team != "" {
		teamVal = team
	}
	_, err := store.Exec(`
		INSERT INTO metric_snapshots (id, team, format, window_start, window_end, payload, created_at)
		VALUES (?, ?, 'json', ?, ?, ?, ?)
	`,
		id, teamVal,
		start.UTC().Format(time.RFC3339),
		createdAt.UTC().Format(time.RFC3339),
		payload,
		createdAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		t.Fatalf("seedSnapshotAt: %v", err)
	}
}

func TestG9DORAHistory_ReturnsLast12Snapshots(t *testing.T) {
	store := setupG9DB(t)
	defer store.Close()
	mux := newG9Mux(store)

	// 15 snapshots, ages descending so we know which 12 are newest.
	for i := 0; i < 15; i++ {
		seedSnapshotAt(t, store, "", float64(i*24), doraPayload(float64(i+1), 2.5, 5.0, 1.0))
	}
	status, body := g9Get(t, mux, "/api/g9/dora/history")
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, body)
	}
	var resp struct {
		DeployFreq []float64 `json:"deploy_freq"`
		Count      int       `json:"count"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, body)
	}
	if resp.Count != 12 {
		t.Errorf("count=%d want 12", resp.Count)
	}
	if len(resp.DeployFreq) != 12 {
		t.Errorf("deploy_freq len=%d want 12", len(resp.DeployFreq))
	}
}

func TestG9DORAHistory_OrderedAscending(t *testing.T) {
	store := setupG9DB(t)
	defer store.Close()
	mux := newG9Mux(store)

	// 3 snapshots, ages 72h / 48h / 24h. After ascending sort, deploy_freq
	// should match age-72h first, age-24h last.
	seedSnapshotAt(t, store, "", 72, doraPayload(1.0, 0, 0, 0))
	seedSnapshotAt(t, store, "", 48, doraPayload(2.0, 0, 0, 0))
	seedSnapshotAt(t, store, "", 24, doraPayload(3.0, 0, 0, 0))

	_, body := g9Get(t, mux, "/api/g9/dora/history")
	var resp struct {
		DeployFreq []float64 `json:"deploy_freq"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	want := []float64{1.0, 2.0, 3.0}
	for i, v := range want {
		if i >= len(resp.DeployFreq) || resp.DeployFreq[i] != v {
			t.Errorf("deploy_freq[%d]=%v want %v (full %v)", i, resp.DeployFreq[i], v, resp.DeployFreq)
		}
	}
}

func TestG9DORAHistory_FewerThan12_ReturnsAvailable(t *testing.T) {
	store := setupG9DB(t)
	defer store.Close()
	mux := newG9Mux(store)

	for i := 0; i < 5; i++ {
		seedSnapshotAt(t, store, "", float64(i*24), doraPayload(1, 1, 1, 1))
	}
	_, body := g9Get(t, mux, "/api/g9/dora/history")
	var resp struct {
		Count        int  `json:"count"`
		Insufficient bool `json:"insufficient"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Count != 5 || resp.Insufficient {
		t.Errorf("count=%d insufficient=%v want 5/false", resp.Count, resp.Insufficient)
	}
}

func TestG9DORAHistory_OneSnapshot_ReturnsHintFlag(t *testing.T) {
	store := setupG9DB(t)
	defer store.Close()
	mux := newG9Mux(store)

	seedSnapshotAt(t, store, "", 1, doraPayload(1, 1, 1, 1))
	_, body := g9Get(t, mux, "/api/g9/dora/history")
	var resp struct {
		Count        int  `json:"count"`
		Insufficient bool `json:"insufficient"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !resp.Insufficient || resp.Count != 1 {
		t.Errorf("got count=%d insufficient=%v want 1/true", resp.Count, resp.Insufficient)
	}
}

func TestG9DORAHistory_ProjectScope_FiltersByTeam(t *testing.T) {
	store := setupG9DB(t)
	defer store.Close()
	mux := newG9Mux(store)

	// 4 snapshots for project A, 4 for B; scope=project&id=A returns only A.
	for i := 0; i < 4; i++ {
		seedSnapshotAt(t, store, "PROJ-A", float64(i*24), doraPayload(float64(10+i), 0, 0, 0))
		seedSnapshotAt(t, store, "PROJ-B", float64(i*24), doraPayload(float64(20+i), 0, 0, 0))
	}
	_, body := g9Get(t, mux, "/api/g9/dora/history?scope=project&id=PROJ-A")
	var resp struct {
		DeployFreq []float64 `json:"deploy_freq"`
		Count      int       `json:"count"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Count != 4 {
		t.Errorf("count=%d want 4", resp.Count)
	}
	for _, v := range resp.DeployFreq {
		if v < 10 || v > 13 {
			t.Errorf("deploy_freq value %v not in PROJ-A range 10-13", v)
		}
	}
}

func TestG9DORAHistory_EmptySnapshots_NoError(t *testing.T) {
	store := setupG9DB(t)
	defer store.Close()
	mux := newG9Mux(store)

	status, body := g9Get(t, mux, "/api/g9/dora/history")
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, body)
	}
	var resp struct {
		Count        int  `json:"count"`
		Insufficient bool `json:"insufficient"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !resp.Insufficient || resp.Count != 0 {
		t.Errorf("got count=%d insufficient=%v want 0/true", resp.Count, resp.Insufficient)
	}
}

// ---- /api/g9/mix-leaderboard ----

func TestG9MixLeaderboard_EmptyDB_ReturnsEmptyList(t *testing.T) {
	store := setupG9DB(t)
	defer store.Close()
	mux := newG9Mux(store)

	status, body := g9Get(t, mux, "/api/g9/mix-leaderboard")
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, body)
	}
	var resp struct {
		Rows []map[string]any `json:"rows"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Rows) != 0 {
		t.Errorf("expected 0 rows, got %d: %v", len(resp.Rows), resp.Rows)
	}
}

func TestG9MixLeaderboard_ReturnsTopN(t *testing.T) {
	store := setupG9DB(t)
	defer store.Close()
	mux := newG9Mux(store)

	// 25 distinct engineers; default limit=20 should trim.
	now := time.Now()
	for i := 0; i < 25; i++ {
		seedRunWithAgent(t, store, fmt.Sprintf("run-%02d", i),
			fmt.Sprintf("eng-%02d", i), "agent-x", float64(i+1), now.AddDate(0, 0, -1))
	}
	_, body := g9Get(t, mux, "/api/g9/mix-leaderboard")
	var resp struct {
		Rows  []map[string]any `json:"rows"`
		Limit int              `json:"limit"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Rows) > resp.Limit {
		t.Errorf("rows=%d > limit=%d", len(resp.Rows), resp.Limit)
	}
	if resp.Limit != 20 {
		t.Errorf("limit=%d want 20", resp.Limit)
	}
}

func TestG9MixLeaderboard_PeriodFilter_Honored(t *testing.T) {
	store := setupG9DB(t)
	defer store.Close()
	mux := newG9Mux(store)

	now := time.Now()
	// Inside default 28d window:
	seedRunWithAgent(t, store, "run-recent", "alice", "agent-x", 1.0, now.AddDate(0, 0, -7))
	// Outside default 28d window:
	seedRunWithAgent(t, store, "run-old", "bob", "agent-x", 1.0, now.AddDate(0, 0, -90))

	_, body := g9Get(t, mux, "/api/g9/mix-leaderboard?period=28")
	var resp struct {
		Rows []struct {
			Engineer string `json:"engineer"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, r := range resp.Rows {
		if r.Engineer == "bob" {
			t.Errorf("bob (90d ago) should be filtered out by 28d window: %v", resp.Rows)
		}
	}
	foundAlice := false
	for _, r := range resp.Rows {
		if r.Engineer == "alice" {
			foundAlice = true
		}
	}
	if !foundAlice {
		t.Errorf("alice (7d ago) should be present; rows=%v", resp.Rows)
	}
}

func TestG9MixLeaderboard_ShapeContainsCoreFields(t *testing.T) {
	store := setupG9DB(t)
	defer store.Close()
	mux := newG9Mux(store)

	now := time.Now()
	seedRunWithAgent(t, store, "run-shape-1", "alice", "agent-x", 2.5, now.AddDate(0, 0, -1))

	_, body := g9Get(t, mux, "/api/g9/mix-leaderboard")
	var resp struct {
		Rows []map[string]any `json:"rows"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, body)
	}
	if len(resp.Rows) == 0 {
		t.Fatalf("expected at least 1 row, got 0")
	}
	r := resp.Rows[0]
	for _, key := range []string{"engineer", "agent", "run_count", "total_cost"} {
		if _, ok := r[key]; !ok {
			t.Errorf("row missing key %q: %v", key, r)
		}
	}
}

func TestG9Alerts_DrilldownURL(t *testing.T) {
	store := setupG9DB(t)
	defer store.Close()
	mux := newG9Mux(store)

	now := time.Now()
	seedRunWithAgent(t, store, "run-hot-2", "alice", "hot-agent", 50.0, now.AddDate(0, 0, -1))
	seedRunWithAgent(t, store, "run-cool-2", "bob", "cool-agent", 10.0, now.AddDate(0, 0, -1))

	_, body := g9Get(t, mux, "/api/g9/alerts")
	var resp struct {
		Alerts []map[string]any `json:"alerts"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, a := range resp.Alerts {
		url, ok := a["drilldown_url"].(string)
		if !ok || url == "" {
			t.Errorf("alert missing drilldown_url: %+v", a)
			continue
		}
		// Cost-multiple drilldown should target an agent scope; AC-dip targets engineer.
		if !strings.Contains(url, "role=") || !strings.Contains(url, "id=") {
			t.Errorf("drilldown_url %q lacks role/id params", url)
		}
	}
}
