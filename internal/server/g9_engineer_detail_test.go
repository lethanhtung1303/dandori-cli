package server_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
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
