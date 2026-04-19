package quality

import (
	"os/exec"
	"strconv"
	"strings"
)

// GitStats holds git diff statistics between two commits
type GitStats struct {
	LinesAdded   int
	LinesRemoved int
	FilesChanged int
	CommitCount  int
	CommitMsgs   []string
}

// GitAnalyzer provides git repository analysis
type GitAnalyzer struct {
	cwd string
}

// NewGitAnalyzer creates a new git analyzer for the given directory
func NewGitAnalyzer(cwd string) *GitAnalyzer {
	return &GitAnalyzer{cwd: cwd}
}

// DiffStats returns lines added/removed between two commits
func (g *GitAnalyzer) DiffStats(before, after string) (*GitStats, error) {
	stats := &GitStats{}

	if before == "" || after == "" || before == after {
		return stats, nil
	}

	// Get numstat for lines added/removed per file
	cmd := exec.Command("git", "diff", "--numstat", before+".."+after)
	cmd.Dir = g.cwd
	out, err := cmd.Output()
	if err != nil {
		// May fail if commits not found; return empty stats
		return stats, nil
	}

	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 {
			// Handle binary files (shown as -)
			if fields[0] != "-" {
				added, _ := strconv.Atoi(fields[0])
				stats.LinesAdded += added
			}
			if fields[1] != "-" {
				removed, _ := strconv.Atoi(fields[1])
				stats.LinesRemoved += removed
			}
			stats.FilesChanged++
		}
	}

	// Count commits between before and after
	cmd = exec.Command("git", "rev-list", "--count", before+".."+after)
	cmd.Dir = g.cwd
	out, err = cmd.Output()
	if err == nil {
		stats.CommitCount, _ = strconv.Atoi(strings.TrimSpace(string(out)))
	}

	// Get commit messages
	if stats.CommitCount > 0 {
		cmd = exec.Command("git", "log", "--format=%s", before+".."+after)
		cmd.Dir = g.cwd
		out, _ = cmd.Output()
		msgs := strings.TrimSpace(string(out))
		if msgs != "" {
			stats.CommitMsgs = strings.Split(msgs, "\n")
		}
	}

	return stats, nil
}

// IsGitRepo checks if the directory is a git repository
func (g *GitAnalyzer) IsGitRepo() bool {
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	cmd.Dir = g.cwd
	err := cmd.Run()
	return err == nil
}

// CurrentHead returns the current HEAD commit hash
func (g *GitAnalyzer) CurrentHead() string {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = g.cwd
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
