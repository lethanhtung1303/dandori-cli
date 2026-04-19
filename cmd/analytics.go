package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/phuc-nt/dandori-cli/internal/db"
	"github.com/spf13/cobra"
)

var analyticsCmd = &cobra.Command{
	Use:   "analytics",
	Short: "Local analytics from SQLite",
	Long: `Query analytics from local SQLite database.
Works offline with multi-agent data on single machine.

Examples:
  dandori analytics                    # Overview
  dandori analytics agents             # Agent performance
  dandori analytics agents --compare alpha,beta
  dandori analytics cost               # Cost breakdown
  dandori analytics cost --group-by task
  dandori analytics sprint 4           # Sprint summary
  dandori analytics runs               # Recent runs`,
	RunE: runAnalyticsOverview,
}

var analyticsCostCmd = &cobra.Command{
	Use:   "cost",
	Short: "Cost breakdown by dimension",
	RunE:  runAnalyticsCost,
}

var analyticsAgentsCmd = &cobra.Command{
	Use:   "agents",
	Short: "Agent stats and comparison",
	RunE:  runAnalyticsAgents,
}

var analyticsSprintCmd = &cobra.Command{
	Use:   "sprint [sprint-id]",
	Short: "Sprint summary",
	Args:  cobra.ExactArgs(1),
	RunE:  runAnalyticsSprint,
}

var analyticsRunsCmd = &cobra.Command{
	Use:   "runs",
	Short: "Recent runs",
	RunE:  runAnalyticsRuns,
}

var analyticsQualityCmd = &cobra.Command{
	Use:   "quality",
	Short: "Code quality metrics by agent",
	Long: `Compare agent code quality based on lint/test deltas.

Quality metrics tracked per run:
  - Lint delta: change in lint errors (negative = improvement)
  - Tests delta: change in passing tests (positive = improvement)
  - Improved %: percentage of runs that improved quality

Examples:
  dandori analytics quality
  dandori analytics quality --compare alpha,beta`,
	RunE: runAnalyticsQuality,
}

var (
	analyticsGroupBy string
	analyticsCompare string
	analyticsFormat  string
	analyticsLimit   int
)

func init() {
	rootCmd.AddCommand(analyticsCmd)
	analyticsCmd.AddCommand(analyticsCostCmd)
	analyticsCmd.AddCommand(analyticsAgentsCmd)
	analyticsCmd.AddCommand(analyticsSprintCmd)
	analyticsCmd.AddCommand(analyticsRunsCmd)
	analyticsCmd.AddCommand(analyticsQualityCmd)

	analyticsCostCmd.Flags().StringVar(&analyticsGroupBy, "group-by", "agent", "Group by: agent, task, day, sprint")
	analyticsCostCmd.Flags().StringVar(&analyticsFormat, "format", "table", "Output format: table, json")

	analyticsAgentsCmd.Flags().StringVar(&analyticsCompare, "compare", "", "Compare specific agents (comma-separated)")
	analyticsAgentsCmd.Flags().StringVar(&analyticsFormat, "format", "table", "Output format: table, json")

	analyticsRunsCmd.Flags().IntVar(&analyticsLimit, "limit", 20, "Number of runs to show")
	analyticsRunsCmd.Flags().StringVar(&analyticsFormat, "format", "table", "Output format: table, json")

	analyticsQualityCmd.Flags().StringVar(&analyticsCompare, "compare", "", "Compare specific agents (comma-separated)")
	analyticsQualityCmd.Flags().StringVar(&analyticsFormat, "format", "table", "Output format: table, json")

	analyticsCmd.PersistentFlags().StringVar(&analyticsFormat, "format", "table", "Output format: table, json")
}

func getLocalDB() (*db.LocalDB, error) {
	return db.Open("")
}

func runAnalyticsOverview(cmd *cobra.Command, args []string) error {
	store, err := getLocalDB()
	if err != nil {
		return err
	}
	defer store.Close()

	runCount, totalCost, totalTokens, err := store.GetTotalStats()
	if err != nil {
		return fmt.Errorf("get stats: %w", err)
	}

	if analyticsFormat == "json" {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"runs":   runCount,
			"cost":   totalCost,
			"tokens": totalTokens,
		})
	}

	fmt.Println("=== Analytics Overview ===")
	fmt.Printf("Total Runs:   %d\n", runCount)
	fmt.Printf("Total Cost:   $%.2f\n", totalCost)
	fmt.Printf("Total Tokens: %d\n", totalTokens)

	if runCount == 0 {
		fmt.Println("\nNo runs recorded yet. Use 'dandori run' to track agent executions.")
	}

	return nil
}

func runAnalyticsCost(cmd *cobra.Command, args []string) error {
	store, err := getLocalDB()
	if err != nil {
		return err
	}
	defer store.Close()

	var groups []db.LocalCostGroup
	switch analyticsGroupBy {
	case "task":
		groups, err = store.GetCostByTask()
	case "day":
		groups, err = store.GetCostByDay()
	case "sprint":
		groups, err = store.GetCostBySprint()
	default:
		groups, err = store.GetCostByAgent()
	}
	if err != nil {
		return fmt.Errorf("get cost: %w", err)
	}

	if len(groups) == 0 {
		fmt.Println("No cost data yet.")
		return nil
	}

	if analyticsFormat == "json" {
		return json.NewEncoder(os.Stdout).Encode(groups)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "GROUP\tCOST\tRUNS\tTOKENS")
	fmt.Fprintln(w, "-----\t----\t----\t------")
	for _, g := range groups {
		fmt.Fprintf(w, "%s\t$%.2f\t%d\t%d\n", g.Group, g.Cost, g.RunCount, g.Tokens)
	}
	return w.Flush()
}

func runAnalyticsAgents(cmd *cobra.Command, args []string) error {
	store, err := getLocalDB()
	if err != nil {
		return err
	}
	defer store.Close()

	stats, err := store.GetAgentStats()
	if err != nil {
		return fmt.Errorf("get agent stats: %w", err)
	}

	// Filter if --compare specified
	if analyticsCompare != "" {
		agents := strings.Split(analyticsCompare, ",")
		agentSet := make(map[string]bool)
		for _, a := range agents {
			agentSet[strings.TrimSpace(a)] = true
		}
		var filtered []db.LocalAgentStat
		for _, s := range stats {
			if agentSet[s.AgentName] {
				filtered = append(filtered, s)
			}
		}
		stats = filtered
	}

	if len(stats) == 0 {
		fmt.Println("No agent data yet.")
		return nil
	}

	if analyticsFormat == "json" {
		return json.NewEncoder(os.Stdout).Encode(stats)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "AGENT\tRUNS\tSUCCESS\tCOST\tAVG COST\tAVG DUR\tTOKENS")
	fmt.Fprintln(w, "-----\t----\t-------\t----\t--------\t-------\t------")
	for _, s := range stats {
		fmt.Fprintf(w, "%s\t%d\t%.1f%%\t$%.2f\t$%.2f\t%.0fs\t%d\n",
			s.AgentName, s.RunCount, s.SuccessRate, s.TotalCost, s.AvgCost, s.AvgDuration, s.TotalTokens)
	}
	return w.Flush()
}

func runAnalyticsSprint(cmd *cobra.Command, args []string) error {
	sprintID := args[0]

	store, err := getLocalDB()
	if err != nil {
		return err
	}
	defer store.Close()

	summary, err := store.GetSprintSummary(sprintID)
	if err != nil {
		return fmt.Errorf("get sprint: %w", err)
	}

	if analyticsFormat == "json" {
		return json.NewEncoder(os.Stdout).Encode(summary)
	}

	fmt.Printf("=== Sprint %s ===\n", sprintID)
	fmt.Printf("Tasks:   %d\n", summary.TaskCount)
	fmt.Printf("Runs:    %d\n", summary.RunCount)
	fmt.Printf("Success: %.1f%%\n", summary.SuccessRate)
	fmt.Printf("Cost:    $%.2f\n", summary.TotalCost)
	fmt.Printf("Tokens:  %d\n", summary.TotalTokens)

	if len(summary.Agents) > 0 {
		fmt.Println("\nBy Agent:")
		for agent, cost := range summary.Agents {
			fmt.Printf("  %s: $%.2f\n", agent, cost)
		}
	}

	return nil
}

func runAnalyticsRuns(cmd *cobra.Command, args []string) error {
	store, err := getLocalDB()
	if err != nil {
		return err
	}
	defer store.Close()

	runs, err := store.GetRecentRuns(analyticsLimit)
	if err != nil {
		return fmt.Errorf("get runs: %w", err)
	}

	if len(runs) == 0 {
		fmt.Println("No runs yet.")
		return nil
	}

	if analyticsFormat == "json" {
		return json.NewEncoder(os.Stdout).Encode(runs)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tTASK\tAGENT\tSTATUS\tDURATION\tCOST")
	fmt.Fprintln(w, "--\t----\t-----\t------\t--------\t----")
	for _, r := range runs {
		task := r.JiraIssueKey
		if task == "" {
			task = "-"
		}
		id := r.ID
		if len(id) > 8 {
			id = id[:8]
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%.0fs\t$%.2f\n",
			id, task, r.AgentName, r.Status, r.Duration, r.Cost)
	}
	return w.Flush()
}

func runAnalyticsQuality(cmd *cobra.Command, args []string) error {
	store, err := getLocalDB()
	if err != nil {
		return err
	}
	defer store.Close()

	stats, err := store.GetQualityStatsByAgent()
	if err != nil {
		return fmt.Errorf("get quality stats: %w", err)
	}

	// Filter if --compare specified
	if analyticsCompare != "" {
		agents := strings.Split(analyticsCompare, ",")
		agentSet := make(map[string]bool)
		for _, a := range agents {
			agentSet[strings.TrimSpace(a)] = true
		}
		var filtered []db.QualityStats
		for _, s := range stats {
			if agentSet[s.AgentName] {
				filtered = append(filtered, s)
			}
		}
		stats = filtered
	}

	if len(stats) == 0 {
		fmt.Println("No quality data yet. Run agents with quality tracking enabled.")
		return nil
	}

	if analyticsFormat == "json" {
		return json.NewEncoder(os.Stdout).Encode(stats)
	}

	fmt.Println("=== Agent Quality Comparison ===")
	fmt.Println("Lint Δ: negative = fewer errors | Tests Δ: positive = more passing")
	fmt.Println()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "AGENT\tRUNS\tLINT Δ\tTESTS Δ\tLINES\tCOMMITS\tMSG QUAL\tIMPROVED")
	fmt.Fprintln(w, "-----\t----\t------\t-------\t-----\t-------\t--------\t--------")
	for _, s := range stats {
		lintSign := ""
		if s.AvgLintDelta > 0 {
			lintSign = "+"
		}
		testsSign := ""
		if s.AvgTestsDelta > 0 {
			testsSign = "+"
		}
		fmt.Fprintf(w, "%s\t%d\t%s%.1f\t%s%.1f\t%.0f\t%d\t%.0f%%\t%.0f%%\n",
			s.AgentName, s.RunCount,
			lintSign, s.AvgLintDelta,
			testsSign, s.AvgTestsDelta,
			s.AvgLinesChanged,
			s.TotalCommits,
			s.AvgCommitQuality*100,
			s.ImprovedPercent)
	}
	return w.Flush()
}
