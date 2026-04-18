package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/phuc-nt/dandori-cli/internal/db"
	"github.com/spf13/cobra"
)

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Local analytics from SQLite (no server required)",
	Long: `View analytics from local SQLite database.
Unlike 'analytics' command which queries the server, 'stats' works offline.

Examples:
  dandori stats              # Overview
  dandori stats agents       # Agent performance
  dandori stats cost         # Cost breakdown
  dandori stats runs         # Recent runs`,
	RunE: runStatsOverview,
}

var statsAgentsCmd = &cobra.Command{
	Use:   "agents",
	Short: "Agent performance stats",
	RunE:  runStatsAgents,
}

var statsCostCmd = &cobra.Command{
	Use:   "cost",
	Short: "Cost breakdown",
	RunE:  runStatsCost,
}

var statsRunsCmd = &cobra.Command{
	Use:   "runs",
	Short: "Recent runs",
	RunE:  runStatsRuns,
}

var (
	statsGroupBy string
	statsLimit   int
	statsJSON    bool
)

func init() {
	rootCmd.AddCommand(statsCmd)
	statsCmd.AddCommand(statsAgentsCmd)
	statsCmd.AddCommand(statsCostCmd)
	statsCmd.AddCommand(statsRunsCmd)

	statsCostCmd.Flags().StringVar(&statsGroupBy, "group-by", "agent", "Group by: agent, task, day")
	statsRunsCmd.Flags().IntVar(&statsLimit, "limit", 20, "Number of runs to show")
	statsCmd.PersistentFlags().BoolVar(&statsJSON, "json", false, "Output as JSON")
}

func getLocalStore() (*db.LocalDB, error) {
	// Open uses default path from config if empty string passed
	return db.Open("")
}

func runStatsOverview(cmd *cobra.Command, args []string) error {
	store, err := getLocalStore()
	if err != nil {
		return err
	}
	defer store.Close()

	runCount, totalCost, totalTokens, err := store.GetTotalStats()
	if err != nil {
		return fmt.Errorf("get stats: %w", err)
	}

	if statsJSON {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"runs":   runCount,
			"cost":   totalCost,
			"tokens": totalTokens,
		})
	}

	fmt.Println("=== Local Statistics ===")
	fmt.Printf("Total Runs:   %d\n", runCount)
	fmt.Printf("Total Cost:   $%.2f\n", totalCost)
	fmt.Printf("Total Tokens: %d\n", totalTokens)

	if runCount == 0 {
		fmt.Println("\nNo runs recorded yet. Use 'dandori run' to track agent executions.")
	}

	return nil
}

func runStatsAgents(cmd *cobra.Command, args []string) error {
	store, err := getLocalStore()
	if err != nil {
		return err
	}
	defer store.Close()

	stats, err := store.GetAgentStats()
	if err != nil {
		return fmt.Errorf("get agent stats: %w", err)
	}

	if len(stats) == 0 {
		fmt.Println("No agent data yet.")
		return nil
	}

	if statsJSON {
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

func runStatsCost(cmd *cobra.Command, args []string) error {
	store, err := getLocalStore()
	if err != nil {
		return err
	}
	defer store.Close()

	var groups []db.LocalCostGroup
	switch statsGroupBy {
	case "task":
		groups, err = store.GetCostByTask()
	case "day":
		groups, err = store.GetCostByDay()
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

	if statsJSON {
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

func runStatsRuns(cmd *cobra.Command, args []string) error {
	store, err := getLocalStore()
	if err != nil {
		return err
	}
	defer store.Close()

	runs, err := store.GetRecentRuns(statsLimit)
	if err != nil {
		return fmt.Errorf("get runs: %w", err)
	}

	if len(runs) == 0 {
		fmt.Println("No runs yet.")
		return nil
	}

	if statsJSON {
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
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%.0fs\t$%.2f\n",
			r.ID[:8], task, r.AgentName, r.Status, r.Duration, r.Cost)
	}
	return w.Flush()
}
