// +build integration

package jira

import (
	"os"
	"testing"
)

func getIntegrationClient(t *testing.T) *Client {
	baseURL := os.Getenv("DANDORI_JIRA_URL")
	user := os.Getenv("DANDORI_JIRA_USER")
	token := os.Getenv("DANDORI_JIRA_TOKEN")

	if baseURL == "" || user == "" || token == "" {
		t.Skip("DANDORI_JIRA_URL, DANDORI_JIRA_USER, DANDORI_JIRA_TOKEN required")
	}

	return NewClient(ClientConfig{
		BaseURL: baseURL,
		User:    user,
		Token:   token,
		IsCloud: true,
	})
}

func TestIntegration_GetIssue(t *testing.T) {
	client := getIntegrationClient(t)

	issue, err := client.GetIssue("CLITEST-1")
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}

	if issue.Key != "CLITEST-1" {
		t.Errorf("key = %s, want CLITEST-1", issue.Key)
	}
	t.Logf("Issue: %s - %s", issue.Key, issue.Summary)
}

func TestIntegration_GetBoards(t *testing.T) {
	client := getIntegrationClient(t)

	boards, err := client.GetBoards("CLITEST")
	if err != nil {
		t.Fatalf("GetBoards: %v", err)
	}

	if len(boards) == 0 {
		t.Error("expected at least 1 board")
	}
	for _, b := range boards {
		t.Logf("Board: %d - %s", b.ID, b.Name)
	}
}

func TestIntegration_GetActiveSprint(t *testing.T) {
	client := getIntegrationClient(t)

	boardID := 3 // CLITEST board
	sprint, err := client.GetActiveSprint(boardID)
	if err != nil {
		t.Fatalf("GetActiveSprint: %v", err)
	}

	if sprint == nil {
		t.Log("No active sprint")
		return
	}
	t.Logf("Sprint: %d - %s (%s)", sprint.ID, sprint.Name, sprint.State)
}

func TestIntegration_GetSprintIssues(t *testing.T) {
	client := getIntegrationClient(t)

	sprintID := 4 // CLITEST sprint
	issues, err := client.GetSprintIssues(sprintID)
	if err != nil {
		t.Fatalf("GetSprintIssues: %v", err)
	}

	t.Logf("Found %d issues in sprint", len(issues))
	for _, iss := range issues {
		t.Logf("  %s - %s", iss.Key, iss.Summary)
	}
}

func TestIntegration_SearchIssues(t *testing.T) {
	client := getIntegrationClient(t)

	issues, err := client.SearchIssues("project = CLITEST", 10)
	if err != nil {
		t.Fatalf("SearchIssues: %v", err)
	}

	if len(issues) == 0 {
		t.Error("expected at least 1 issue")
	}
	t.Logf("Found %d issues", len(issues))
}

func TestIntegration_AddComment(t *testing.T) {
	client := getIntegrationClient(t)

	err := client.AddComment("CLITEST-1", "Integration test comment from dandori-cli")
	if err != nil {
		t.Fatalf("AddComment: %v", err)
	}
	t.Log("Comment added successfully")
}
