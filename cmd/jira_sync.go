package cmd

import (
	"fmt"
	"log/slog"
	"strings"
	"unicode/utf8"

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

		// Add completion comment (G8: include intent sections when available).
		comment := formatRunCommentWithStore(run, store)
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
	return formatRunCommentWithStore(run, nil)
}

// formatRunCommentWithStore builds the Jira wiki-markup comment for a finished
// run. When store is non-nil it queries G8 intent events and appends the
// ### Intent and ### Key Decisions sections. Passing nil store (or when the
// query fails) produces the same output as the legacy v0.5.0 format — the
// caller is never blocked.
func formatRunCommentWithStore(run db.SyncableRun, store *db.LocalDB) string {
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

	// G8: append intent + decision sections when store is available.
	if store != nil {
		appendG8Sections(&sb, store, run.ID)
	}

	return sb.String()
}

// appendG8Sections queries layer-4 G8 events for runID and appends the
// ### Intent and ### Key Decisions blocks to sb. It is fail-soft: any error
// is logged at Warn and no partial sections are written.
func appendG8Sections(sb *strings.Builder, store *db.LocalDB, runID string) {
	events, err := store.GetIntentEvents(runID)
	if err != nil {
		slog.Warn("g8: query intent events failed — skipping intent sections",
			"run_id", runID, "error", err)
		return
	}
	if events.Intent == nil {
		// Legacy run or intent extraction disabled — skip both sections.
		return
	}

	sb.WriteString("\n\n")
	sb.WriteString("h3. Intent\n")

	firstMsg := truncateJira(events.Intent.FirstUserMsg, 400)
	if firstMsg != "" {
		sb.WriteString(fmt.Sprintf("{quote}%s{quote}\n", firstMsg))
	}

	summary := truncateJira(events.Intent.Summary, 400)
	if summary != "" {
		sb.WriteString(fmt.Sprintf("*Summary:* %s\n", summary))
	}

	// Spec links: Jira back-pointer + Confluence URLs.
	specParts := buildSpecLine(events.Intent.SpecLinks)
	if specParts != "" {
		sb.WriteString(fmt.Sprintf("*Specs:* %s\n", specParts))
	}

	// Key Decisions section — only when decisions are present.
	if len(events.Decisions) > 0 {
		sb.WriteString("\n")
		sb.WriteString("h3. Key Decisions\n")
		for i, d := range events.Decisions {
			sb.WriteString(fmt.Sprintf("%d. *Chose:* %s\n", i+1, d.Chosen))
			if len(d.Rejected) > 0 {
				sb.WriteString(fmt.Sprintf("   *Over:* %s\n", strings.Join(d.Rejected, ", ")))
			}
			if r := truncateJira(d.Rationale, 200); r != "" {
				sb.WriteString(fmt.Sprintf("   _Reason: %s_ _(heuristic)_\n", r))
			} else {
				sb.WriteString("   _(heuristic)_\n")
			}
		}
	}
}

// buildSpecLine constructs the spec reference line from SpecLinks.
// Returns "" when there is nothing to show.
func buildSpecLine(sl db.IntentSpecLinks) string {
	var parts []string
	if sl.JiraKey != "" {
		parts = append(parts, sl.JiraKey)
	}
	for _, u := range sl.ConfluenceURLs {
		parts = append(parts, fmt.Sprintf("[Confluence|%s]", u))
	}
	return strings.Join(parts, " · ")
}

// truncateJira truncates s to at most maxChars Unicode code points.
// If truncated, an ellipsis is appended. Returns the original string when it
// is already within the limit.
func truncateJira(s string, maxChars int) string {
	if utf8.RuneCountInString(s) <= maxChars {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxChars]) + "…"
}
