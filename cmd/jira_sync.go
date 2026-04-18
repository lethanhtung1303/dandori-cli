package cmd

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/phuc-nt/dandori-cli/internal/db"
	"github.com/phuc-nt/dandori-cli/internal/jira"
	"github.com/spf13/cobra"
)

var jiraSyncCmd = &cobra.Command{
	Use:   "jira-sync",
	Short: "Sync run status to Jira",
	Long: `Sync completed runs to Jira by updating issue status and adding comments.

For each completed run:
- Status "done" → Transitions issue to "Done" + adds completion comment
- Status "failed" → Adds failure comment (keeps current status)

Examples:
  dandori jira-sync              # Sync all unsynced completed runs
  dandori jira-sync --dry-run    # Preview without making changes
  dandori jira-sync --task CLITEST-1  # Sync specific task only`,
	RunE: runJiraSync,
}

var (
	jiraSyncDryRun bool
	jiraSyncTask   string
)

func init() {
	rootCmd.AddCommand(jiraSyncCmd)
	jiraSyncCmd.Flags().BoolVar(&jiraSyncDryRun, "dry-run", false, "Preview changes without applying")
	jiraSyncCmd.Flags().StringVar(&jiraSyncTask, "task", "", "Sync specific task only")
}

func runJiraSync(cmd *cobra.Command, args []string) error {
	cfg := Config()
	if cfg == nil || cfg.Jira.BaseURL == "" {
		return fmt.Errorf("jira not configured - run 'dandori init' first")
	}

	store, err := db.Open("")
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer store.Close()

	jiraClient := jira.NewClient(jira.ClientConfig{
		BaseURL: cfg.Jira.BaseURL,
		User:    cfg.Jira.User,
		Token:   cfg.Jira.Token,
		IsCloud: cfg.Jira.Cloud,
	})

	// Get completed runs that haven't been synced
	runs, err := store.GetRunsToSync(jiraSyncTask)
	if err != nil {
		return fmt.Errorf("get runs: %w", err)
	}

	if len(runs) == 0 {
		fmt.Println("No runs to sync.")
		return nil
	}

	fmt.Printf("Found %d runs to sync:\n\n", len(runs))

	synced := 0
	for _, run := range runs {
		if run.JiraIssueKey == "" {
			continue
		}

		fmt.Printf("  %s (%s) - %s\n", run.JiraIssueKey, run.AgentName, run.Status)

		if jiraSyncDryRun {
			fmt.Printf("    [dry-run] Would transition to %s and add comment\n", statusToJira(run.Status))
			continue
		}

		// Transition issue based on run status
		if run.Status == "done" {
			if err := jiraClient.TransitionToDone(run.JiraIssueKey, jira.DefaultStatusMapping); err != nil {
				slog.Warn("transition failed", "task", run.JiraIssueKey, "error", err)
				// Continue anyway - might already be in correct status
			}
		}

		// Add completion comment
		comment := formatRunComment(run)
		if err := jiraClient.AddComment(run.JiraIssueKey, comment); err != nil {
			slog.Warn("add comment failed", "task", run.JiraIssueKey, "error", err)
		}

		// Mark as synced
		if err := store.MarkRunSynced(run.ID); err != nil {
			slog.Warn("mark synced failed", "run", run.ID, "error", err)
		}

		synced++
		fmt.Printf("    ✓ Synced to Jira\n")
	}

	if jiraSyncDryRun {
		fmt.Printf("\n[dry-run] Would sync %d runs\n", len(runs))
	} else {
		fmt.Printf("\nSynced %d/%d runs to Jira\n", synced, len(runs))
	}

	return nil
}

func statusToJira(status string) string {
	switch status {
	case "done":
		return "Done"
	case "failed":
		return "To Do (with failure comment)"
	default:
		return "In Progress"
	}
}

func formatRunComment(run db.SyncableRun) string {
	var sb strings.Builder

	if run.Status == "done" {
		sb.WriteString("✅ *Agent Run Completed*\n\n")
	} else {
		sb.WriteString("❌ *Agent Run Failed*\n\n")
	}

	sb.WriteString(fmt.Sprintf("*Agent:* %s\n", run.AgentName))
	sb.WriteString(fmt.Sprintf("*Duration:* %.0fs\n", run.Duration))
	sb.WriteString(fmt.Sprintf("*Cost:* $%.2f\n", run.Cost))
	sb.WriteString(fmt.Sprintf("*Tokens:* %d\n", run.Tokens))

	if run.Status == "done" {
		sb.WriteString("\n_Task completed by AI agent._")
	} else {
		sb.WriteString("\n_Run failed - may need manual intervention._")
	}

	return sb.String()
}
