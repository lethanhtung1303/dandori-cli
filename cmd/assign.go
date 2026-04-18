package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/phuc-nt/dandori-cli/internal/assignment"
	"github.com/phuc-nt/dandori-cli/internal/config"
	"github.com/phuc-nt/dandori-cli/internal/jira"
	"github.com/spf13/cobra"
)

var assignCmd = &cobra.Command{
	Use:   "assign",
	Short: "Manage agent assignments for Jira tasks",
	Long: `Assign agents to Jira tasks.

Examples:
  dandori assign suggest PROJ-123     # Get agent suggestion for task
  dandori assign set PROJ-123 alpha   # Manually assign agent to task
  dandori assign list                  # List pending assignments`,
}

var assignSuggestCmd = &cobra.Command{
	Use:   "suggest <issue-key>",
	Short: "Suggest best agent for a task",
	Args:  cobra.ExactArgs(1),
	RunE:  runAssignSuggest,
}

var assignSetCmd = &cobra.Command{
	Use:   "set <issue-key> <agent-name>",
	Short: "Manually assign agent to task",
	Args:  cobra.ExactArgs(2),
	RunE:  runAssignSet,
}

var assignListCmd = &cobra.Command{
	Use:   "list",
	Short: "List pending assignments",
	RunE:  runAssignList,
}

func init() {
	assignCmd.AddCommand(assignSuggestCmd)
	assignCmd.AddCommand(assignSetCmd)
	assignCmd.AddCommand(assignListCmd)
	rootCmd.AddCommand(assignCmd)
}

func runAssignSuggest(cmd *cobra.Command, args []string) error {
	issueKey := args[0]

	cfg := Config()
	if cfg == nil || cfg.Jira.BaseURL == "" {
		return fmt.Errorf("jira not configured")
	}

	// Fetch issue from Jira
	jiraClient := jira.NewClient(jira.ClientConfig{
		BaseURL: cfg.Jira.BaseURL,
		User:    cfg.Jira.User,
		Token:   cfg.Jira.Token,
		IsCloud: cfg.Jira.Cloud,
	})

	issue, err := jiraClient.GetIssue(issueKey)
	if err != nil {
		return fmt.Errorf("get issue: %w", err)
	}

	task := assignment.Task{
		IssueKey:   issue.Key,
		Summary:    issue.Summary,
		IssueType:  issue.IssueType,
		Priority:   issue.Priority,
		Labels:     issue.Labels,
		Components: []string{}, // TODO: extract components
	}

	// Fetch agents from server
	agents, err := fetchAgentsFromServer(cfg.ServerURL)
	if err != nil {
		return fmt.Errorf("fetch agents: %w", err)
	}

	if len(agents) == 0 {
		fmt.Println("No agents registered. Use 'dandori init' on workstations to register agents.")
		return nil
	}

	// Run suggestion engine
	engine := assignment.NewEngine(nil) // No history for local suggest
	suggestions := engine.Suggest(task, agents)

	if len(suggestions) == 0 {
		fmt.Println("No suitable agents found.")
		return nil
	}

	fmt.Printf("Suggestions for %s (%s):\n\n", issueKey, issue.Summary)
	for i, s := range suggestions {
		marker := " "
		if i == 0 {
			marker = "→"
		}
		fmt.Printf("%s %s: %d%% — %s\n", marker, s.AgentName, s.Score, s.Reason)
	}

	fmt.Printf("\nTo assign: dandori assign set %s %s\n", issueKey, suggestions[0].AgentName)
	return nil
}

func runAssignSet(cmd *cobra.Command, args []string) error {
	issueKey := args[0]
	agentName := args[1]

	cfg := Config()
	if cfg == nil || cfg.Jira.BaseURL == "" {
		return fmt.Errorf("jira not configured")
	}

	// Set agent field in Jira (via comment for now)
	jiraClient := jira.NewClient(jira.ClientConfig{
		BaseURL: cfg.Jira.BaseURL,
		User:    cfg.Jira.User,
		Token:   cfg.Jira.Token,
		IsCloud: cfg.Jira.Cloud,
	})

	comment := fmt.Sprintf("🤖 Agent assigned: **%s**\n\nTo start work: `dandori run --task %s`", agentName, issueKey)
	if err := jiraClient.AddComment(issueKey, comment); err != nil {
		return fmt.Errorf("add comment: %w", err)
	}

	fmt.Printf("Assigned %s to %s\n", agentName, issueKey)
	fmt.Printf("Comment added to Jira issue.\n")

	// TODO: Also update server assignment status
	return nil
}

func runAssignList(cmd *cobra.Command, args []string) error {
	cfg := Config()
	if cfg == nil || cfg.ServerURL == "" {
		return fmt.Errorf("server_url not configured")
	}

	resp, err := http.Get(cfg.ServerURL + "/api/assignments?status=pending")
	if err != nil {
		return fmt.Errorf("fetch assignments: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Assignments []struct {
			ID             int64   `json:"ID"`
			JiraIssueKey   string  `json:"JiraIssueKey"`
			SuggestedAgent *string `json:"SuggestedAgent"`
			SuggestedScore *int    `json:"SuggestedScore"`
			Status         string  `json:"Status"`
		} `json:"assignments"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	if len(result.Assignments) == 0 {
		fmt.Println("No pending assignments.")
		return nil
	}

	fmt.Println("Pending assignments:")
	for _, a := range result.Assignments {
		agent := "-"
		score := "-"
		if a.SuggestedAgent != nil {
			agent = *a.SuggestedAgent
		}
		if a.SuggestedScore != nil {
			score = fmt.Sprintf("%d%%", *a.SuggestedScore)
		}
		fmt.Printf("  %s: %s (%s)\n", a.JiraIssueKey, agent, score)
	}
	return nil
}

func fetchAgentsFromServer(serverURL string) ([]assignment.AgentConfig, error) {
	if serverURL == "" {
		return nil, nil
	}

	resp, err := http.Get(serverURL + "/api/agents?active=true")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Agents []struct {
			Name                string   `json:"Name"`
			AgentType           string   `json:"AgentType"`
			Capabilities        []string `json:"Capabilities"`
			PreferredIssueTypes []string `json:"PreferredIssueTypes"`
			MaxConcurrent       int      `json:"MaxConcurrent"`
			Active              bool     `json:"Active"`
		} `json:"agents"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	agents := make([]assignment.AgentConfig, 0, len(result.Agents))
	for _, a := range result.Agents {
		agents = append(agents, assignment.AgentConfig{
			Name:                a.Name,
			AgentType:           a.AgentType,
			Capabilities:        a.Capabilities,
			PreferredIssueTypes: a.PreferredIssueTypes,
			MaxConcurrent:       a.MaxConcurrent,
			Active:              a.Active,
		})
	}
	return agents, nil
}

// fetchAgentsFromConfig returns agent from local config as fallback
func fetchAgentsFromConfig(cfg *config.Config) []assignment.AgentConfig {
	if cfg.Agent.Name == "" {
		return nil
	}

	caps := cfg.Agent.Capabilities
	if caps == nil {
		caps = []string{}
	}

	return []assignment.AgentConfig{{
		Name:          cfg.Agent.Name,
		AgentType:     cfg.Agent.Type,
		Capabilities:  caps,
		MaxConcurrent: 3,
		Active:        true,
	}}
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if strings.EqualFold(s, item) {
			return true
		}
	}
	return false
}
