package quality

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestGitAnalyzer_DiffStats(t *testing.T) {
	// Create temp git repo for testing
	tmpDir, err := os.MkdirTemp("", "git-analyzer-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Init git repo
	runGit(t, tmpDir, "init")
	runGit(t, tmpDir, "config", "user.email", "test@test.com")
	runGit(t, tmpDir, "config", "user.name", "Test")

	// Create initial commit
	writeFile(t, tmpDir, "file1.txt", "line1\nline2\n")
	runGit(t, tmpDir, "add", ".")
	runGit(t, tmpDir, "commit", "-m", "feat: initial commit")
	before := getHead(t, tmpDir)

	// Make changes
	writeFile(t, tmpDir, "file1.txt", "line1\nline2\nline3\n")
	writeFile(t, tmpDir, "file2.txt", "new file\n")
	runGit(t, tmpDir, "add", ".")
	runGit(t, tmpDir, "commit", "-m", "feat: add more content")
	after := getHead(t, tmpDir)

	// Test DiffStats
	analyzer := NewGitAnalyzer(tmpDir)
	stats, err := analyzer.DiffStats(before, after)
	if err != nil {
		t.Fatalf("DiffStats error: %v", err)
	}

	if stats.LinesAdded < 2 {
		t.Errorf("LinesAdded = %d, want >= 2", stats.LinesAdded)
	}
	if stats.FilesChanged != 2 {
		t.Errorf("FilesChanged = %d, want 2", stats.FilesChanged)
	}
	if stats.CommitCount != 1 {
		t.Errorf("CommitCount = %d, want 1", stats.CommitCount)
	}
	if len(stats.CommitMsgs) != 1 {
		t.Errorf("CommitMsgs len = %d, want 1", len(stats.CommitMsgs))
	}
}

func TestGitAnalyzer_EmptyDiff(t *testing.T) {
	analyzer := NewGitAnalyzer(".")

	// Same commit
	stats, err := analyzer.DiffStats("abc123", "abc123")
	if err != nil {
		t.Fatalf("DiffStats error: %v", err)
	}
	if stats.LinesAdded != 0 || stats.FilesChanged != 0 {
		t.Error("Expected zero stats for same commit")
	}

	// Empty commits
	stats, _ = analyzer.DiffStats("", "")
	if stats.LinesAdded != 0 {
		t.Error("Expected zero stats for empty commits")
	}
}

func TestScoreCommitMessages(t *testing.T) {
	tests := []struct {
		name     string
		msgs     []string
		minScore float64
		maxScore float64
	}{
		{
			name:     "empty messages",
			msgs:     []string{},
			minScore: 0,
			maxScore: 0,
		},
		{
			name:     "conventional commit",
			msgs:     []string{"feat: Add user authentication system"},
			minScore: 0.7,
			maxScore: 1.0,
		},
		{
			name:     "conventional with scope",
			msgs:     []string{"fix(auth): Resolve token expiration bug"},
			minScore: 0.7,
			maxScore: 1.0,
		},
		{
			name:     "with issue reference",
			msgs:     []string{"feat: Add login page PROJ-123"},
			minScore: 0.6,
			maxScore: 1.0,
		},
		{
			name:     "poor WIP commit",
			msgs:     []string{"WIP"},
			minScore: 0,
			maxScore: 0.3,
		},
		{
			name:     "fixup commit",
			msgs:     []string{"fixup! previous commit"},
			minScore: 0,
			maxScore: 0.3,
		},
		{
			name:     "multiple mixed quality",
			msgs:     []string{"feat: Good commit message", "wip", "fix: Another good one"},
			minScore: 0.4,
			maxScore: 0.8,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := ScoreCommitMessages(tt.msgs)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("ScoreCommitMessages() = %.2f, want [%.2f, %.2f]",
					score, tt.minScore, tt.maxScore)
			}
		})
	}
}

// Helper functions
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git %v failed: %v", args, err)
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func getHead(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	return string(out[:len(out)-1])
}
