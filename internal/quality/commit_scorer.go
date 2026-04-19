package quality

import (
	"regexp"
	"strings"
)

// Conventional commit pattern: type(scope): description
var conventionalPattern = regexp.MustCompile(`^(feat|fix|docs|style|refactor|test|chore|perf|ci|build|revert)(\([^)]+\))?: .+`)

// Issue reference pattern: PROJ-123 or #123
var issuePattern = regexp.MustCompile(`([A-Z]+-\d+|#\d+)`)

// ScoreCommitMessages returns 0-1 quality score for commit messages
func ScoreCommitMessages(msgs []string) float64 {
	if len(msgs) == 0 {
		return 0
	}

	var total float64
	for _, msg := range msgs {
		total += scoreMessage(msg)
	}
	return total / float64(len(msgs))
}

// scoreMessage scores a single commit message (0-1)
func scoreMessage(msg string) float64 {
	if msg == "" {
		return 0
	}

	var score float64

	// Conventional commit format: +0.4
	if conventionalPattern.MatchString(msg) {
		score += 0.4
	}

	// Reasonable length (10-72 chars): +0.2
	if len(msg) >= 10 && len(msg) <= 72 {
		score += 0.2
	} else if len(msg) >= 5 && len(msg) <= 100 {
		score += 0.1 // Partial credit
	}

	// Starts with capital letter after colon (for conventional): +0.1
	if idx := strings.Index(msg, ": "); idx > 0 && idx+2 < len(msg) {
		desc := msg[idx+2:]
		if len(desc) > 0 && desc[0] >= 'A' && desc[0] <= 'Z' {
			score += 0.1
		}
	}

	// No WIP/fixup/squash markers: +0.2
	lower := strings.ToLower(msg)
	if !strings.Contains(lower, "wip") &&
		!strings.Contains(lower, "fixup") &&
		!strings.Contains(lower, "squash") &&
		!strings.HasPrefix(lower, "tmp") &&
		!strings.HasPrefix(lower, "temp") {
		score += 0.2
	}

	// References an issue: +0.1
	if issuePattern.MatchString(msg) {
		score += 0.1
	}

	// Cap at 1.0
	if score > 1.0 {
		score = 1.0
	}

	return score
}
