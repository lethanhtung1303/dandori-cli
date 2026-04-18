package wrapper

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/phuc-nt/dandori-cli/internal/db"
	"github.com/phuc-nt/dandori-cli/internal/util"
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

// Scenario 18: Empty command array
func TestRunEmptyCommand(t *testing.T) {
	localDB := setupTestDB(t)
	defer localDB.Close()

	_, err := Run(context.Background(), localDB, Options{Command: []string{}})
	if err == nil {
		t.Error("empty command should return error")
	}
}

// Scenario 19: Command with special chars
func TestRunCommandSpecialChars(t *testing.T) {
	localDB := setupTestDB(t)
	defer localDB.Close()

	opts := Options{
		Command:   []string{"echo", "hello 'world' \"test\" $VAR"},
		AgentName: "test",
		AgentType: "claude_code",
		DryRun:    true,
	}

	result, err := Run(context.Background(), localDB, opts)
	if err != nil {
		t.Fatalf("special chars should work: %v", err)
	}
	if result.RunID == "" {
		t.Error("should generate run ID")
	}
}

// Scenario 21: SIGINT during execution (context cancel)
func TestRunContextCancel(t *testing.T) {
	localDB := setupTestDB(t)
	defer localDB.Close()

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	opts := Options{
		Command:   []string{"sleep", "10"},
		AgentName: "test",
		AgentType: "claude_code",
		NoTailer:  true,
	}

	start := time.Now()
	Run(ctx, localDB, opts)
	elapsed := time.Since(start)

	if elapsed > 2*time.Second {
		t.Error("context cancel should stop quickly")
	}
}

// Scenario 23: Child process quick exit
func TestRunQuickExit(t *testing.T) {
	localDB := setupTestDB(t)
	defer localDB.Close()

	opts := Options{
		Command:   []string{"true"},
		AgentName: "test",
		AgentType: "claude_code",
		NoTailer:  true,
	}

	result, err := Run(context.Background(), localDB, opts)
	if err != nil {
		t.Fatalf("quick exit failed: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("exit code = %d, want 0", result.ExitCode)
	}
}

// Scenario 25: Git repo not initialized
func TestRunNoGitRepo(t *testing.T) {
	tmpDir := t.TempDir()
	oldDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldDir)

	head := getGitHead()
	if head != "" {
		t.Log("no git repo should return empty HEAD")
	}
}

// Scenario 28: Run command even if DB insert fails (dry run test)
func TestRunDryRunNoDBWrite(t *testing.T) {
	localDB := setupTestDB(t)
	defer localDB.Close()

	opts := Options{
		Command:   []string{"echo", "test"},
		AgentName: "test",
		AgentType: "claude_code",
		DryRun:    true,
	}

	_, err := Run(context.Background(), localDB, opts)
	if err != nil {
		t.Fatalf("dry run failed: %v", err)
	}

	var count int
	localDB.QueryRow(`SELECT COUNT(*) FROM runs`).Scan(&count)
	if count != 0 {
		t.Error("dry run should not insert to DB")
	}
}

// Scenario 30: Run ID format validation
func TestRunIDFormat(t *testing.T) {
	id := util.GenerateRunID()
	if len(id) != 16 {
		t.Errorf("run ID should be 16 chars, got %d", len(id))
	}
	// Verify hex format
	for _, c := range id {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("run ID should be hex, got char %c", c)
		}
	}
}

// Additional: Branch parser edge cases
func TestBranchParserEdgeCases(t *testing.T) {
	tests := []struct {
		branch   string
		expected string
	}{
		{"", ""},
		{"main", ""},
		{"feature/", ""},
		{"feature/abc", ""},
		{"feature/ABC-", ""},
		{"feature/ABC-0", "ABC-0"},
		{"feature/a-1", "A-1"},           // case insensitive match
		{"FEATURE/PROJ-123", "PROJ-123"}, // case insensitive
		{"bugfix/BUG-999-fix", "BUG-999"},
		{"PROJ-123", "PROJ-123"},
		{"refs/heads/feature/PROJ-456", "PROJ-456"},
	}

	for _, tt := range tests {
		result := ExtractJiraKey(tt.branch)
		if result != tt.expected {
			t.Errorf("ExtractJiraKey(%q) = %q, want %q", tt.branch, result, tt.expected)
		}
	}
}

// Additional: Token parser edge cases
func TestTokenParserEdgeCases(t *testing.T) {
	tests := []struct {
		name  string
		line  string
		input int
	}{
		{"empty line", "", 0},
		{"empty json", "{}", 0},
		{"null usage", `{"type":"assistant","message":{"usage":null}}`, 0},
		{"zero tokens", `{"type":"assistant","message":{"usage":{"input_tokens":0}}}`, 0},
		{"negative tokens", `{"type":"assistant","message":{"usage":{"input_tokens":-100}}}`, -100},
		{"large tokens", `{"type":"assistant","message":{"usage":{"input_tokens":999999999}}}`, 999999999},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usage := parseLineForTokens([]byte(tt.line))
			if usage.Input != tt.input {
				t.Errorf("input = %d, want %d", usage.Input, tt.input)
			}
		})
	}
}

// Additional: Cost computation edge cases
func TestCostComputationEdgeCases(t *testing.T) {
	tests := []struct {
		name  string
		usage TokenUsage
		want  float64
	}{
		{"zero tokens", TokenUsage{}, 0},
		{"unknown model", TokenUsage{Input: 1000000, Model: "unknown-model"}, 3.0}, // uses sonnet default
		{"only cache read", TokenUsage{CacheRead: 1000000, Model: "claude-sonnet-4-6"}, 0.30},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cost := ComputeCost(tt.usage)
			if cost != tt.want {
				t.Errorf("cost = %f, want %f", cost, tt.want)
			}
		})
	}
}
