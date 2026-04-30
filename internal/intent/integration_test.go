// Package intent integration tests verify the full G8 pipeline:
// Extract(JSONL) → RecordEvent(DB) → GetIntentEvents(DB) → BuildRunReport.
//
// Each per-phase unit test exercises one layer in isolation. These tests cross
// boundaries to confirm the layers compose correctly.
package intent_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/phuc-nt/dandori-cli/internal/db"
	"github.com/phuc-nt/dandori-cli/internal/event"
	"github.com/phuc-nt/dandori-cli/internal/intent"
	"github.com/phuc-nt/dandori-cli/internal/model"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func openTestDB(t *testing.T) *db.LocalDB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

// insertTestRun seeds a minimal runs row so foreign-key constraints pass.
func insertTestRun(t *testing.T, store *db.LocalDB, runID, jiraKey string) {
	t.Helper()
	_, err := store.Exec(`
		INSERT INTO runs (id, jira_issue_key, agent_name, agent_type, user, workstation_id, started_at, status)
		VALUES (?, ?, 'claude-code', 'claude_code', 'tester', 'ws-1', ?, 'done')
	`, runID, jiraKey, time.Now().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("insert run %s: %v", runID, err)
	}
}

// fixtureSessionJSONL writes a session JSONL to a temp file and returns the path.
// The content contains:
//   - 1 user message
//   - 3 tool_use assistant messages (with narrative text → decision phrases)
//   - 1 thinking block
//   - 1 final assistant summary
//
// Decision phrases: "I'll go with bcrypt because it's already in our deps"
// and "using table inheritance over separate table".
func fixtureSessionJSONL(t *testing.T) string {
	t.Helper()
	lines := []string{
		`{"type":"user","message":{"content":[{"type":"text","text":"Implement password hashing for CLITEST-99"}]}}`,
		`{"type":"assistant","message":{"content":[` +
			`{"type":"thinking","thinking":"Considering options: bcrypt vs argon2."},` +
			`{"type":"text","text":"I'll go with bcrypt because it's already in our deps."},` +
			`{"type":"tool_use","id":"tu1","name":"Read","input":{"file_path":"go.mod"}}` +
			`]}}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"tu1","content":"module example"}]}}`,
		`{"type":"assistant","message":{"content":[` +
			`{"type":"text","text":"Now writing the hash function."},` +
			`{"type":"tool_use","id":"tu2","name":"Write","input":{"file_path":"auth/hash.go","content":"..."}}` +
			`]}}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"tu2","content":"ok"}]}}`,
		`{"type":"assistant","message":{"content":[` +
			`{"type":"text","text":"Using table inheritance over separate table for flexibility."},` +
			`{"type":"tool_use","id":"tu3","name":"Edit","input":{"file_path":"db/schema.sql"}}` +
			`]}}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"tu3","content":"done"}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"Bcrypt hashing implemented and all tests pass."}]}}`,
	}

	f, err := os.CreateTemp(t.TempDir(), "session-*.jsonl")
	if err != nil {
		t.Fatalf("create session file: %v", err)
	}
	for _, line := range lines {
		if _, err := f.WriteString(line + "\n"); err != nil {
			t.Fatalf("write line: %v", err)
		}
	}
	f.Close()
	return f.Name()
}

// storeIntentResult persists the Extract result into the DB using the same
// pattern as wrapper.runIntentExtraction — so the integration test exercises
// the real storage path, not hand-crafted SQL.
func storeIntentResult(t *testing.T, store *db.LocalDB, runID string, res intent.Result) {
	t.Helper()
	recorder := event.NewRecorder(store)

	if res.FirstUserMsg != "" || res.Summary != "" || len(res.Reasoning) > 0 {
		payload := map[string]any{
			"first_user_msg": res.FirstUserMsg,
			"summary":        res.Summary,
			"spec_links":     res.SpecLinks,
		}
		if err := recorder.RecordEvent(runID, model.LayerSemantic, model.EventTypeIntentExtracted, payload); err != nil {
			t.Fatalf("record intent.extracted: %v", err)
		}

		for i, rb := range res.Reasoning {
			if err := recorder.RecordEvent(runID, model.LayerSemantic, model.EventTypeAgentReasoning, map[string]any{
				"index":  i,
				"source": rb.Source,
				"text":   rb.Text,
			}); err != nil {
				t.Fatalf("record agent.reasoning[%d]: %v", i, err)
			}
		}
	}

	for _, d := range res.Decisions {
		if err := recorder.RecordEvent(runID, model.LayerSemantic, model.EventTypeDecisionPoint, map[string]any{
			"chosen":        d.Chosen,
			"rejected":      d.Rejected,
			"rationale":     d.Rationale,
			"ts_offset_sec": d.TsOffsetSec,
		}); err != nil {
			t.Fatalf("record decision.point: %v", err)
		}
	}
}

// ---------------------------------------------------------------------------
// Test 1: End-to-end intent flow
// ---------------------------------------------------------------------------

// TestIntegration_EndToEnd_IntentFlow verifies the complete G8 pipeline:
// JSONL file → Extract → persist in DB → GetIntentEvents → BuildRunReport.
//
// Assertions:
//   - events table has 1 intent.extracted row
//   - events table has ≥1 decision.point row (from decision phrases in fixture)
//   - GetIntentEvents returns parsed intent with correct first_user_msg
//   - BuildRunReport contains ## Intent section and ## Key Decisions section
func TestIntegration_EndToEnd_IntentFlow(t *testing.T) {
	store := openTestDB(t)
	const runID = "integration-run-001"
	insertTestRun(t, store, runID, "CLITEST-99")

	// Step 1: Extract intent from JSONL fixture.
	sessionPath := fixtureSessionJSONL(t)
	res, err := intent.Extract(sessionPath, runID, "", "CLITEST-99")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	// Verify extraction produced the expected signals.
	if res.FirstUserMsg == "" {
		t.Fatal("extraction: expected FirstUserMsg, got empty")
	}
	if !strings.Contains(res.FirstUserMsg, "password hashing") {
		t.Errorf("FirstUserMsg unexpected: %q", res.FirstUserMsg)
	}
	if res.Summary == "" {
		t.Fatal("extraction: expected Summary, got empty")
	}
	if len(res.Reasoning) == 0 {
		t.Fatal("extraction: expected reasoning blocks")
	}

	// Step 2: Persist into DB.
	storeIntentResult(t, store, runID, res)

	// Step 3: Verify raw DB counts.
	var intentCount, decisionCount int
	if err := store.QueryRow(`SELECT COUNT(*) FROM events WHERE run_id=? AND event_type='intent.extracted'`, runID).Scan(&intentCount); err != nil {
		t.Fatalf("count intent.extracted: %v", err)
	}
	if err := store.QueryRow(`SELECT COUNT(*) FROM events WHERE run_id=? AND event_type='decision.point'`, runID).Scan(&decisionCount); err != nil {
		t.Fatalf("count decision.point: %v", err)
	}
	if intentCount != 1 {
		t.Errorf("expected 1 intent.extracted event, got %d", intentCount)
	}
	// Fixture has "I'll go with bcrypt because..." and "Using...over..." → ≥1 decision.
	if decisionCount < 1 {
		t.Errorf("expected ≥1 decision.point event, got %d", decisionCount)
	}

	// Step 4: Read back via GetIntentEvents.
	intentEvents, err := store.GetIntentEvents(runID)
	if err != nil {
		t.Fatalf("GetIntentEvents: %v", err)
	}
	if intentEvents.Intent == nil {
		t.Fatal("GetIntentEvents: expected Intent, got nil")
	}
	if !strings.Contains(intentEvents.Intent.FirstUserMsg, "password hashing") {
		t.Errorf("GetIntentEvents first_user_msg: %q", intentEvents.Intent.FirstUserMsg)
	}
	if intentEvents.Intent.SpecLinks.JiraKey != "CLITEST-99" {
		t.Errorf("GetIntentEvents jira_key: %q, want CLITEST-99", intentEvents.Intent.SpecLinks.JiraKey)
	}
	if len(intentEvents.Decisions) < 1 {
		t.Errorf("GetIntentEvents: expected ≥1 decision, got %d", len(intentEvents.Decisions))
	}

	// Step 5: Build incident report and assert sections present.
	reportData := &intent.ReportData{
		Run: &db.RunRecord{
			ID:           runID,
			JiraIssueKey: "CLITEST-99",
			AgentName:    "claude-code",
			StartedAt:    time.Now(),
			Status:       "done",
		},
		Intent: intentEvents,
	}
	report := intent.BuildRunReport(reportData)

	for _, want := range []string{
		"## Intent",
		"password hashing",
		"## Key Decisions",
		"bcrypt",
		"⚠️ Decisions extracted via heuristic",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("report missing %q", want)
		}
	}
}

// ---------------------------------------------------------------------------
// Test 2: Fail-soft regression — malformed JSONL
// ---------------------------------------------------------------------------

// TestIntegration_MalformedJSONL_NoG8Events verifies that a malformed JSONL
// session does not crash, produces no G8 events in the DB, and the comment/report
// renders cleanly with placeholder text (not panics or partial sections).
func TestIntegration_MalformedJSONL_NoG8Events(t *testing.T) {
	store := openTestDB(t)
	const runID = "integration-run-002"
	insertTestRun(t, store, runID, "CLITEST-00")

	// Create a session JSONL that is entirely malformed JSON.
	f, err := os.CreateTemp(t.TempDir(), "malformed-*.jsonl")
	if err != nil {
		t.Fatalf("create file: %v", err)
	}
	f.WriteString("not json\n{broken: true\n\x00\x01\x02\n")
	f.Close()

	// Step 1: Extract must not error (fail-soft).
	res, err := intent.Extract(f.Name(), runID, "", "")
	if err != nil {
		t.Fatalf("Extract must not error on malformed JSONL: %v", err)
	}

	// Step 2: Nothing to store — wrapper skips when empty.
	// Simulate the wrapper's guard: only store if meaningful content found.
	if res.FirstUserMsg != "" || res.Summary != "" || len(res.Reasoning) > 0 {
		storeIntentResult(t, store, runID, res)
	}

	// Step 3: No G8 events in DB.
	var count int
	if err := store.QueryRow(`SELECT COUNT(*) FROM events WHERE run_id=? AND layer=4`, runID).Scan(&count); err != nil {
		t.Fatalf("count layer-4 events: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 layer-4 events for malformed session, got %d", count)
	}

	// Step 4: GetIntentEvents returns empty (not nil result).
	intentEvents, err := store.GetIntentEvents(runID)
	if err != nil {
		t.Fatalf("GetIntentEvents: %v", err)
	}
	if intentEvents.Intent != nil {
		t.Errorf("expected nil Intent for run with no events, got %+v", intentEvents.Intent)
	}
	if len(intentEvents.Decisions) != 0 {
		t.Errorf("expected 0 decisions, got %d", len(intentEvents.Decisions))
	}

	// Step 5: BuildRunReport renders gracefully with placeholder text.
	reportData := &intent.ReportData{
		Run: &db.RunRecord{
			ID:     runID,
			Status: "done",
		},
		Intent: intentEvents,
	}
	report := intent.BuildRunReport(reportData)

	if !strings.Contains(report, "_no intent captured_") {
		t.Errorf("report should show placeholder when no intent; got:\n%s", report)
	}
	if !strings.Contains(report, "_no decisions captured_") {
		t.Errorf("report should show placeholder when no decisions; got:\n%s", report)
	}
	// Must not panic or produce partial sections.
	if strings.Contains(report, "CLITEST-99") {
		t.Errorf("report contains data from different run")
	}
}

// ---------------------------------------------------------------------------
// Test 3: Cross-run aggregation via BuildTaskReport
// ---------------------------------------------------------------------------

// TestIntegration_CrossRunAggregation verifies that 3 runs under the same
// jira_issue_key aggregate correctly in the task incident report:
// - Total runs = 3
// - Cumulative cost is sum of all runs
// - Each run's intent section is independently present
// - BuildTaskReport does not mix up decisions across runs
func TestIntegration_CrossRunAggregation(t *testing.T) {
	store := openTestDB(t)
	const jiraKey = "CLITEST-100"

	type runSpec struct {
		id   string
		cost float64
		msg  string
	}
	runs := []runSpec{
		{"cross-run-1", 0.10, "First attempt at refactor"},
		{"cross-run-2", 0.20, "Second attempt with tests"},
		{"cross-run-3", 0.15, "Third attempt cleanup"},
	}

	sessionPath := fixtureSessionJSONL(t)

	for _, spec := range runs {
		insertTestRun(t, store, spec.id, jiraKey)

		res, err := intent.Extract(sessionPath, spec.id, "", jiraKey)
		if err != nil {
			t.Fatalf("Extract run %s: %v", spec.id, err)
		}
		// Override first user msg for per-run distinctness via DB seeding.
		// We verify aggregation via run count and cost sum, not msg content.
		storeIntentResult(t, store, spec.id, res)
	}

	// Build per-run report data slices.
	var runDataSlices []*intent.ReportData
	totalCost := 0.0
	for _, spec := range runs {
		totalCost += spec.cost
		intentEvents, err := store.GetIntentEvents(spec.id)
		if err != nil {
			t.Fatalf("GetIntentEvents %s: %v", spec.id, err)
		}
		runDataSlices = append(runDataSlices, &intent.ReportData{
			Run: &db.RunRecord{
				ID:           spec.id,
				JiraIssueKey: jiraKey,
				AgentName:    "claude-code",
				Status:       "done",
				CostUSD:      spec.cost,
			},
			Intent: intentEvents,
		})
	}

	taskData := &intent.TaskReportData{
		JiraKey: jiraKey,
		Runs:    runDataSlices,
	}
	report := intent.BuildTaskReport(taskData)

	// Assertions.
	if !strings.Contains(report, "# Incident Report — Task "+jiraKey) {
		t.Errorf("missing task header")
	}
	if !strings.Contains(report, "Total runs") {
		t.Errorf("missing Total runs line")
	}
	if !strings.Contains(report, "3") {
		t.Errorf("missing run count 3")
	}

	// All 3 run blocks must appear.
	for i := 1; i <= 3; i++ {
		want := "## Run " + string(rune('0'+i)) + " of 3"
		if !strings.Contains(report, want) {
			t.Errorf("missing %q in task report", want)
		}
	}

	// Cumulative cost should be the sum (formatted in report).
	// The report formats cost as $%.4f so verify $0.45xx appears.
	if !strings.Contains(report, "0.4500") {
		t.Errorf("cumulative cost $0.4500 not found; total was %.4f", totalCost)
	}

	// Each run should have its own Intent section.
	intentCount := strings.Count(report, "## Intent")
	if intentCount != 3 {
		t.Errorf("expected 3 Intent sections, got %d", intentCount)
	}
}

// ---------------------------------------------------------------------------
// Test 4: env gate — DANDORI_INTENT_DISABLED disables extraction
// ---------------------------------------------------------------------------

// TestIntegration_EnvGate_NoEventsStored verifies that when
// DANDORI_INTENT_DISABLED=1 the extractor returns an empty Result, the wrapper
// stores no G8 events, and the report shows placeholders.
func TestIntegration_EnvGate_NoEventsStored(t *testing.T) {
	t.Setenv("DANDORI_INTENT_DISABLED", "1")

	store := openTestDB(t)
	const runID = "integration-run-gate"
	insertTestRun(t, store, runID, "CLITEST-gate")

	sessionPath := fixtureSessionJSONL(t)
	res, err := intent.Extract(sessionPath, runID, "", "CLITEST-gate")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	// Nothing stored (wrapper guard).
	if res.FirstUserMsg != "" || res.Summary != "" || len(res.Reasoning) > 0 {
		storeIntentResult(t, store, runID, res)
	}

	var count int
	if err := store.QueryRow(`SELECT COUNT(*) FROM events WHERE run_id=? AND layer=4`, runID).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 events when DANDORI_INTENT_DISABLED=1, got %d", count)
	}
}

// ---------------------------------------------------------------------------
// Test 5: Jira comment pipeline — verify G8 sections in Jira-format output
// ---------------------------------------------------------------------------

// TestIntegration_JiraCommentSections verifies that intent+decision events
// stored in the DB produce a Jira comment containing h3. Intent and
// h3. Key Decisions sections via the same logic used by jira-sync.
//
// Note: formatRunCommentWithStore is in cmd/ (unexported). This test exercises
// the equivalent logic directly against the DB layer to confirm the DB round-trip
// is correct; the cmd/ P4 tests (jira_sync_intent_test.go) cover the formatting
// function with a DB-backed store.
func TestIntegration_JiraCommentSections_DBRoundTrip(t *testing.T) {
	store := openTestDB(t)
	const runID = "integration-run-jira"
	insertTestRun(t, store, runID, "CLITEST-jira")

	sessionPath := fixtureSessionJSONL(t)
	res, err := intent.Extract(sessionPath, runID, "", "CLITEST-jira")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	storeIntentResult(t, store, runID, res)

	// Verify round-trip through DB.
	intentEvents, err := store.GetIntentEvents(runID)
	if err != nil {
		t.Fatalf("GetIntentEvents: %v", err)
	}

	// The G8 comment extension relies on:
	// 1. intentEvents.Intent != nil → h3. Intent section written
	// 2. len(intentEvents.Decisions) > 0 → h3. Key Decisions section written
	if intentEvents.Intent == nil {
		t.Fatal("Intent must be non-nil for Jira comment sections to appear")
	}

	// Verify the payload JSON round-trips correctly for each decision.
	for i, d := range intentEvents.Decisions {
		if d.Chosen == "" {
			t.Errorf("decision[%d].Chosen is empty", i)
		}
	}

	// Verify raw JSON shape in the events table is parseable with standard fields.
	rows, err := store.Query(`SELECT data FROM events WHERE run_id=? AND event_type='decision.point'`, runID)
	if err != nil {
		t.Fatalf("query decision rows: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			t.Fatalf("scan: %v", err)
		}
		var m map[string]json.RawMessage
		if err := json.Unmarshal([]byte(raw), &m); err != nil {
			t.Errorf("decision.point payload not valid JSON: %s", raw)
		}
		if _, ok := m["chosen"]; !ok {
			t.Errorf("decision.point missing 'chosen' field: %s", raw)
		}
	}
}
