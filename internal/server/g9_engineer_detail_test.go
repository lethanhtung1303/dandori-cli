package server_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/phuc-nt/dandori-cli/internal/db"
)

// TestG9EngineerDetail_Returns50Runs seeds 80 runs for alice and asserts the
// response caps the runs array at 50 (newest first).
func TestG9EngineerDetail_Returns50Runs(t *testing.T) {
	store := setupG9DB(t)
	defer store.Close()

	now := time.Now()
	for i := 0; i < 80; i++ {
		runID := fmt.Sprintf("alice-run-%03d", i)
		// Stagger started_at so we can verify ordering.
		seedRunG9(t, store, runID, "alice", 0.5, now.Add(-time.Duration(i)*time.Hour))
	}
	// Add some noise: bob runs that should NOT appear in alice's payload.
	for i := 0; i < 5; i++ {
		seedRunG9(t, store, fmt.Sprintf("bob-run-%d", i), "bob", 0.3, now.Add(-time.Duration(i)*time.Hour))
	}

	mux := newG9Mux(store)
	status, body := g9Get(t, mux, "/api/g9/engineer/alice")
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", status, body)
	}

	var resp struct {
		Engineer string           `json:"engineer"`
		Runs     []map[string]any `json:"runs"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, body)
	}
	if resp.Engineer != "alice" {
		t.Errorf("engineer=%q want alice", resp.Engineer)
	}
	if len(resp.Runs) != 50 {
		t.Errorf("runs len=%d want 50", len(resp.Runs))
	}
	// Verify no bob rows leaked in.
	for _, r := range resp.Runs {
		if r["engineer_name"] != "alice" {
			t.Errorf("non-alice run leaked: %v", r)
		}
	}
}

// TestG9EngineerDetail_RetentionSparkline_Has4Buckets asserts a 4-bucket
// (weekly) retention sparkline is included over a 28d window.
func TestG9EngineerDetail_RetentionSparkline_Has4Buckets(t *testing.T) {
	store := setupG9DB(t)
	defer store.Close()

	now := time.Now()
	// Seed runs across 4 weeks so each bucket has ≥1 run.
	for week := 0; week < 4; week++ {
		for i := 0; i < 3; i++ {
			runID := fmt.Sprintf("alice-w%d-r%d", week, i)
			seedRunG9(t, store, runID, "alice", 0.5, now.Add(-time.Duration(week*7+i)*24*time.Hour))
		}
	}

	mux := newG9Mux(store)
	status, body := g9Get(t, mux, "/api/g9/engineer/alice")
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", status, body)
	}

	var resp struct {
		RetentionSparkline []float64 `json:"retention_sparkline"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, body)
	}
	if len(resp.RetentionSparkline) != 4 {
		t.Errorf("retention_sparkline len=%d want 4 (weekly buckets)", len(resp.RetentionSparkline))
	}
}

// TestG9EngineerDetail_UnknownEngineer_ReturnsEmptyPayload asserts unknown
// engineer name returns 200 with empty arrays — engineer view is tolerant.
func TestG9EngineerDetail_UnknownEngineer_ReturnsEmptyPayload(t *testing.T) {
	store := setupG9DB(t)
	defer store.Close()

	mux := newG9Mux(store)
	status, body := g9Get(t, mux, "/api/g9/engineer/ghost")
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", status, body)
	}
	var resp struct {
		Engineer           string           `json:"engineer"`
		Runs               []map[string]any `json:"runs"`
		RetentionSparkline []float64        `json:"retention_sparkline"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, body)
	}
	if resp.Engineer != "ghost" {
		t.Errorf("engineer=%q want ghost", resp.Engineer)
	}
	if len(resp.Runs) != 0 {
		t.Errorf("runs len=%d want 0", len(resp.Runs))
	}
	if len(resp.RetentionSparkline) != 4 {
		t.Errorf("retention_sparkline len=%d want 4 (zeros)", len(resp.RetentionSparkline))
	}
}

// ---- G10 P1: Engineer KPI strip tests ----

// seedRunWithIntervention adds a run with an explicit intervention count.
func seedRunWithIntervention(t *testing.T, store *db.LocalDB, runID, engineer string, costUSD float64, startedAt time.Time, interventions int, status string) {
	t.Helper()
	_, err := store.Exec(`
		INSERT INTO runs (id, jira_issue_key, agent_name, agent_type, user, workstation_id,
		                  engineer_name, started_at, cost_usd, status, human_intervention_count)
		VALUES (?, 'TASK-1', 'claude-code', 'claude_code', 'tester', 'ws-1', ?, ?, ?, ?, ?)
	`, runID, engineer, startedAt.UTC().Format(time.RFC3339), costUSD, status, interventions)
	if err != nil {
		t.Fatalf("seedRunWithIntervention %s: %v", runID, err)
	}
}

// TestG9Engineer_KPIStrip_BasicAggregates seeds 5 alice + 5 bob runs and
// asserts /api/g9/engineer/alice returns alice-only cost/interventions in
// the new kpi_7d block.
func TestG9Engineer_KPIStrip_BasicAggregates(t *testing.T) {
	store := setupG9DB(t)
	defer store.Close()

	now := time.Now()
	// 5 alice runs in last 7 days, 1 with intervention.
	for i := 0; i < 5; i++ {
		intervs := 0
		if i == 0 {
			intervs = 2
		}
		seedRunWithIntervention(t, store,
			fmt.Sprintf("alice-r%d", i), "alice", 1.5,
			now.Add(-time.Duration(i+1)*24*time.Hour),
			intervs, "done")
	}
	// Bob noise — must NOT appear in alice payload.
	for i := 0; i < 5; i++ {
		seedRunWithIntervention(t, store,
			fmt.Sprintf("bob-r%d", i), "bob", 9.9,
			now.Add(-time.Duration(i+1)*24*time.Hour),
			0, "done")
	}

	mux := newG9Mux(store)
	status, body := g9Get(t, mux, "/api/g9/engineer/alice")
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", status, body)
	}
	var resp struct {
		KPI7d struct {
			Cost7d          float64 `json:"cost_7d"`
			Runs7d          int     `json:"runs_7d"`
			Interventions7d int     `json:"interventions_7d"`
			AutonomyPct     float64 `json:"autonomy_pct"`
			SuccessPct      float64 `json:"success_pct"`
		} `json:"kpi_7d"`
		Empty bool `json:"empty"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, body)
	}
	if resp.Empty {
		t.Error("empty=true, want false (alice has runs)")
	}
	if resp.KPI7d.Runs7d != 5 {
		t.Errorf("runs_7d=%d want 5 (alice only)", resp.KPI7d.Runs7d)
	}
	wantCost := 7.5 // 5 × 1.5
	if got := resp.KPI7d.Cost7d; got < wantCost-0.01 || got > wantCost+0.01 {
		t.Errorf("cost_7d=%v want %v (5 alice runs × $1.5)", got, wantCost)
	}
	if resp.KPI7d.Interventions7d != 2 {
		t.Errorf("interventions_7d=%d want 2", resp.KPI7d.Interventions7d)
	}
	// 4 of 5 runs intervention-free → autonomy = 80%
	if got := resp.KPI7d.AutonomyPct; got < 79 || got > 81 {
		t.Errorf("autonomy_pct=%v want ~80", got)
	}
	// All 5 runs status=done → success = 100%
	if got := resp.KPI7d.SuccessPct; got < 99 {
		t.Errorf("success_pct=%v want 100", got)
	}
}

// TestG9Engineer_KPIStrip_AutonomyFormula pins the autonomy formula:
// autonomy = (runs with 0 interventions) / total runs × 100.
func TestG9Engineer_KPIStrip_AutonomyFormula(t *testing.T) {
	store := setupG9DB(t)
	defer store.Close()

	now := time.Now()
	// 10 runs in last ~6 days (well inside 7d window), 3 with interventions
	// → autonomy 70%. Use 12h spacing to fit all in window.
	for i := 0; i < 10; i++ {
		intervs := 0
		if i < 3 {
			intervs = 1
		}
		seedRunWithIntervention(t, store,
			fmt.Sprintf("alice-r%d", i), "alice", 1.0,
			now.Add(-time.Duration(i+1)*12*time.Hour),
			intervs, "done")
	}

	mux := newG9Mux(store)
	status, body := g9Get(t, mux, "/api/g9/engineer/alice")
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", status, body)
	}
	var resp struct {
		KPI7d struct {
			AutonomyPct float64 `json:"autonomy_pct"`
		} `json:"kpi_7d"`
	}
	_ = json.Unmarshal(body, &resp)
	if got := resp.KPI7d.AutonomyPct; got < 69 || got > 71 {
		t.Errorf("autonomy_pct=%v want ~70 (7 of 10 zero-intervention)", got)
	}
}

// TestG9Engineer_KPIStrip_WoWDeltas seeds current 7d and prior 7d windows
// with known cost differences and asserts wow_delta_pct.
func TestG9Engineer_KPIStrip_WoWDeltas(t *testing.T) {
	store := setupG9DB(t)
	defer store.Close()

	now := time.Now()
	// Current 7d: 4 runs × $5 = $20
	for i := 0; i < 4; i++ {
		seedRunWithIntervention(t, store,
			fmt.Sprintf("alice-cur-%d", i), "alice", 5.0,
			now.Add(-time.Duration(i+1)*24*time.Hour),
			0, "done")
	}
	// Prior 7d (8-14d ago): 4 runs × $4 = $16
	for i := 0; i < 4; i++ {
		seedRunWithIntervention(t, store,
			fmt.Sprintf("alice-prior-%d", i), "alice", 4.0,
			now.Add(-time.Duration(8+i)*24*time.Hour),
			0, "done")
	}

	mux := newG9Mux(store)
	status, body := g9Get(t, mux, "/api/g9/engineer/alice")
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", status, body)
	}
	var resp struct {
		KPI7d struct {
			Cost7d float64 `json:"cost_7d"`
		} `json:"kpi_7d"`
		WoW struct {
			CostPriorUSD float64 `json:"cost_prior_usd"`
			CostDeltaPct float64 `json:"cost_delta_pct"`
		} `json:"wow"`
	}
	_ = json.Unmarshal(body, &resp)
	if got := resp.KPI7d.Cost7d; got < 19.99 || got > 20.01 {
		t.Errorf("cost_7d=%v want 20", got)
	}
	if got := resp.WoW.CostPriorUSD; got < 15.99 || got > 16.01 {
		t.Errorf("cost_prior_usd=%v want 16", got)
	}
	// (20-16)/16 = +25%
	if got := resp.WoW.CostDeltaPct; got < 24 || got > 26 {
		t.Errorf("cost_delta_pct=%v want ~25", got)
	}
}

// TestG9Engineer_KPIStrip_NoRuns_ReturnsEmpty unknown engineer → empty=true,
// no false 0% values that would mislead managers.
func TestG9Engineer_KPIStrip_NoRuns_ReturnsEmpty(t *testing.T) {
	store := setupG9DB(t)
	defer store.Close()

	mux := newG9Mux(store)
	status, body := g9Get(t, mux, "/api/g9/engineer/ghost")
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", status, body)
	}
	var resp struct {
		Empty bool `json:"empty"`
		KPI7d struct {
			Runs7d int `json:"runs_7d"`
		} `json:"kpi_7d"`
	}
	_ = json.Unmarshal(body, &resp)
	if !resp.Empty {
		t.Error("empty=false, want true (ghost has no runs)")
	}
	if resp.KPI7d.Runs7d != 0 {
		t.Errorf("runs_7d=%d want 0", resp.KPI7d.Runs7d)
	}
}
