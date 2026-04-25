//go:build e2e

package integration

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// dandoriBinary resolves the dandori binary path — default repo-root/bin/dandori.
func dandoriBinary(t *testing.T) string {
	t.Helper()
	if p := os.Getenv("DANDORI_BIN"); p != "" {
		return p
	}
	_, thisFile, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	bin := filepath.Join(repoRoot, "bin", "dandori")
	if _, err := os.Stat(bin); err != nil {
		t.Skipf("dandori binary missing at %s (run 'make build' first)", bin)
	}
	return bin
}

// runCLI shells out to the dandori binary and returns combined output.
func runCLI(t *testing.T, bin, dbPath string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), "DANDORI_DB="+dbPath)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}

// TestE2E_Rehearsal_DryRun exercises the 6-step flow without a real Claude
// invocation. Still needs the binary built; no Jira/Confluence connectivity.
func TestE2E_Rehearsal_DryRun(t *testing.T) {
	bin := dandoriBinary(t)
	dbPath := filepath.Join(t.TempDir(), "rehearsal-dry.db")

	start := time.Now()

	if out, err := runCLI(t, bin, dbPath, "demo", "--reset", "--seed"); err != nil {
		t.Fatalf("demo --reset --seed: %v\n%s", err, out)
	}

	allOut, err := runCLI(t, bin, dbPath, "analytics", "all", "--since", "30")
	if err != nil {
		t.Fatalf("analytics all: %v\n%s", err, allOut)
	}
	for _, block := range []string{"COST BY", "LEADERBOARD", "QUALITY GATES", "ALERTS"} {
		if !strings.Contains(allOut, block) {
			t.Errorf("analytics all missing %q block. output:\n%s", block, allOut)
		}
	}

	engOut, err := runCLI(t, bin, dbPath, "analytics", "cost", "--by", "engineer")
	if err != nil {
		t.Fatalf("analytics cost --by engineer: %v\n%s", err, engOut)
	}
	for _, name := range []string{"Alice", "Bob", "Carol"} {
		if !strings.Contains(engOut, name) {
			t.Errorf("cost by engineer missing %q. output:\n%s", name, engOut)
		}
	}

	deptOut, err := runCLI(t, bin, dbPath, "analytics", "cost", "--by", "department")
	if err != nil {
		t.Fatalf("analytics cost --by department: %v\n%s", err, deptOut)
	}
	for _, dept := range []string{"Platform", "Growth"} {
		if !strings.Contains(deptOut, dept) {
			t.Errorf("cost by department missing %q. output:\n%s", dept, deptOut)
		}
	}

	mixOut, err := runCLI(t, bin, dbPath, "analytics", "mix", "--since", "30")
	if err != nil {
		t.Fatalf("analytics mix: %v\n%s", err, mixOut)
	}
	if !strings.Contains(mixOut, "(human)") {
		t.Errorf("expected human-only row marker '(human)'. output:\n%s", mixOut)
	}

	elapsed := time.Since(start)
	if elapsed > 30*time.Second {
		t.Errorf("dry rehearsal exceeded 30s: %v", elapsed)
	}
	t.Logf("dry rehearsal ok in %v", elapsed)
}

// TestE2E_Rehearsal_FullDemoFlow is the live-Claude variant. It posts a real
// run against CLITEST-1 and verifies tokens/cost captured — exercising the
// Phase 01 tailer timing fix end-to-end.
func TestE2E_Rehearsal_FullDemoFlow(t *testing.T) {
	if os.Getenv("DANDORI_E2E_CLAUDE") != "1" {
		t.Skip("set DANDORI_E2E_CLAUDE=1 to run the live-Claude rehearsal")
	}
	for _, k := range []string{"DANDORI_JIRA_URL", "DANDORI_JIRA_USER", "DANDORI_JIRA_TOKEN"} {
		if os.Getenv(k) == "" {
			t.Skipf("%s required", k)
		}
	}
	bin := dandoriBinary(t)
	dbPath := filepath.Join(t.TempDir(), "rehearsal-live.db")

	start := time.Now()

	if out, err := runCLI(t, bin, dbPath, "demo", "--reset", "--seed"); err != nil {
		t.Fatalf("demo --reset --seed: %v\n%s", err, out)
	}

	if out, err := runCLI(t, bin, dbPath, "task", "run", "CLITEST-1"); err != nil {
		t.Fatalf("task run CLITEST-1: %v\n%s", err, out)
	}

	runsOut, err := runCLI(t, bin, dbPath, "analytics", "runs", "--limit", "1")
	if err != nil {
		t.Fatalf("analytics runs: %v\n%s", err, runsOut)
	}
	// Tailer fix: tokens should be non-zero in the last run. Accept either
	// the "0s" duration marker or explicit tokens — crude but avoids JSON
	// coupling since runs output is a fixed table.
	if strings.Contains(runsOut, "CLITEST-1") && strings.Contains(runsOut, "$0.00\t0") {
		t.Errorf("tailer regression: zero cost/tokens on CLITEST-1. output:\n%s", runsOut)
	}

	allOut, err := runCLI(t, bin, dbPath, "analytics", "all", "--since", "30")
	if err != nil {
		t.Fatalf("analytics all: %v\n%s", err, allOut)
	}
	for _, block := range []string{"COST BY", "LEADERBOARD", "QUALITY GATES", "ALERTS"} {
		if !strings.Contains(allOut, block) {
			t.Errorf("analytics all missing %q. output:\n%s", block, allOut)
		}
	}

	elapsed := time.Since(start)
	if elapsed > 180*time.Second {
		t.Errorf("live rehearsal exceeded 180s budget: %v", elapsed)
	}
	t.Logf("live rehearsal ok in %v", elapsed)
}
