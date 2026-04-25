package analytics

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/phuc-nt/dandori-cli/internal/db"
	"github.com/phuc-nt/dandori-cli/internal/demo"
)

func openSeededDB(t *testing.T) *db.LocalDB {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "all.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := d.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := demo.SeedBlogScenario(d); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return d
}

func openEmptyDB(t *testing.T) *db.LocalDB {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "empty.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := d.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return d
}

func TestAnalyticsAll_BuildsFourBlocks(t *testing.T) {
	d := openSeededDB(t)

	snap, err := BuildSnapshot(d, Window{Since: 30 * 24 * time.Hour}, DefaultThresholds())
	if err != nil {
		t.Fatalf("BuildSnapshot: %v", err)
	}
	if len(snap.CostByProject) == 0 {
		t.Error("CostByProject empty")
	}
	if len(snap.Leaderboard) < 3 {
		t.Errorf("Leaderboard: expected >=3 rows, got %d", len(snap.Leaderboard))
	}
	if snap.WindowLabel == "" {
		t.Error("WindowLabel empty")
	}
}

func TestAnalyticsAll_FormatTable_ContainsAllBlocks(t *testing.T) {
	d := openSeededDB(t)
	snap, err := BuildSnapshot(d, Window{Since: 30 * 24 * time.Hour}, DefaultThresholds())
	if err != nil {
		t.Fatal(err)
	}
	out := FormatTable(snap)
	for _, header := range []string{"COST BY", "LEADERBOARD", "QUALITY GATES", "ALERTS"} {
		if !strings.Contains(out, header) {
			t.Errorf("table missing %q block; output:\n%s", header, out)
		}
	}
	for _, name := range []string{"Alice", "Bob", "Carol"} {
		if !strings.Contains(out, name) {
			t.Errorf("table missing %q", name)
		}
	}
}

func TestAnalyticsAll_FormatJSON_Valid(t *testing.T) {
	d := openSeededDB(t)
	snap, err := BuildSnapshot(d, Window{Since: 30 * 24 * time.Hour}, DefaultThresholds())
	if err != nil {
		t.Fatal(err)
	}
	out := FormatJSON(snap)
	var round Snapshot
	if err := json.Unmarshal([]byte(out), &round); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if round.WindowLabel != snap.WindowLabel {
		t.Errorf("round-trip WindowLabel mismatch: %q vs %q", round.WindowLabel, snap.WindowLabel)
	}
}

func TestAnalyticsAll_EmptyDB_NoPanic(t *testing.T) {
	d := openEmptyDB(t)
	snap, err := BuildSnapshot(d, Window{Since: time.Hour}, DefaultThresholds())
	if err != nil {
		t.Fatalf("BuildSnapshot on empty: %v", err)
	}
	if len(snap.Leaderboard) != 0 {
		t.Errorf("expected empty leaderboard, got %+v", snap.Leaderboard)
	}
	if len(snap.Alerts) != 0 {
		t.Errorf("expected no alerts on empty db, got %+v", snap.Alerts)
	}
	// Must still format without panic
	_ = FormatTable(snap)
	_ = FormatJSON(snap)
}

func TestAnalyticsAll_WindowLabel(t *testing.T) {
	if got := (Window{Since: 7 * 24 * time.Hour}).label(); got != "last 7d" {
		t.Errorf("expected 'last 7d', got %q", got)
	}
	if got := (Window{}).label(); got != "last 30d" {
		t.Errorf("expected default 'last 30d', got %q", got)
	}
}

var _ = os.Stdout // keep imports stable if file is edited down
