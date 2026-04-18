package wrapper

import (
	"os/exec"
	"regexp"
	"strings"
)

var (
	branchJiraPattern  = regexp.MustCompile(`(?i)(?:feature|bugfix|hotfix|fix|task)/([A-Z]+-\d+)`)
	generalJiraPattern = regexp.MustCompile(`([A-Z]{2,10}-\d+)`)
)

func ExtractJiraKeyFromBranch() string {
	branch := getCurrentBranch()
	if branch == "" {
		return ""
	}
	return ExtractJiraKey(branch)
}

func ExtractJiraKey(branch string) string {
	if matches := branchJiraPattern.FindStringSubmatch(branch); len(matches) > 1 {
		return strings.ToUpper(matches[1])
	}

	if matches := generalJiraPattern.FindStringSubmatch(branch); len(matches) > 1 {
		return strings.ToUpper(matches[1])
	}

	return ""
}

func getCurrentBranch() string {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
