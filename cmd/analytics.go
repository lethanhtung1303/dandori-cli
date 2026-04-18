package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/phuc-nt/dandori-cli/internal/analytics"
	"github.com/spf13/cobra"
)

var analyticsCmd = &cobra.Command{
	Use:   "analytics",
	Short: "Query analytics from server",
	Long:  "Query agent efficiency, cost attribution, and sprint analytics from the monitoring server.",
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

var (
	analyticsGroupBy string
	analyticsAgent   string
	analyticsSprint  string
	analyticsFrom    string
	analyticsTo      string
	analyticsCompare string
	analyticsFormat  string
)

func init() {
	rootCmd.AddCommand(analyticsCmd)
	analyticsCmd.AddCommand(analyticsCostCmd)
	analyticsCmd.AddCommand(analyticsAgentsCmd)
	analyticsCmd.AddCommand(analyticsSprintCmd)

	analyticsCostCmd.Flags().StringVar(&analyticsGroupBy, "group-by", "agent", "Group by: agent, sprint, task, day, week, month")
	analyticsCostCmd.Flags().StringVar(&analyticsSprint, "sprint", "", "Filter by sprint ID")
	analyticsCostCmd.Flags().StringVar(&analyticsFrom, "from", "", "Start date (YYYY-MM-DD)")
	analyticsCostCmd.Flags().StringVar(&analyticsTo, "to", "", "End date (YYYY-MM-DD)")
	analyticsCostCmd.Flags().StringVar(&analyticsFormat, "format", "table", "Output format: table, json, csv")

	analyticsAgentsCmd.Flags().StringVar(&analyticsCompare, "compare", "", "Compare agents (comma-separated)")
	analyticsAgentsCmd.Flags().StringVar(&analyticsAgent, "agent", "", "Filter by agent")
	analyticsAgentsCmd.Flags().StringVar(&analyticsSprint, "sprint", "", "Filter by sprint ID")
	analyticsAgentsCmd.Flags().StringVar(&analyticsFrom, "from", "", "Start date")
	analyticsAgentsCmd.Flags().StringVar(&analyticsTo, "to", "", "End date")
	analyticsAgentsCmd.Flags().StringVar(&analyticsFormat, "format", "table", "Output format: table, json")
}

func runAnalyticsCost(cmd *cobra.Command, args []string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	params := url.Values{}
	params.Set("group_by", analyticsGroupBy)
	if analyticsSprint != "" {
		params.Set("sprint", analyticsSprint)
	}
	if analyticsFrom != "" {
		params.Set("from", analyticsFrom)
	}
	if analyticsTo != "" {
		params.Set("to", analyticsTo)
	}

	resp, err := http.Get(cfg.ServerURL + "/api/analytics/cost?" + params.Encode())
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server error %d: %s", resp.StatusCode, body)
	}

	var result struct {
		Data []analytics.CostGroup `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode failed: %w", err)
	}

	switch analyticsFormat {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result.Data)
	case "csv":
		return analytics.ExportCSV(result.Data, os.Stdout)
	default:
		return printCostTable(result.Data)
	}
}

func runAnalyticsAgents(cmd *cobra.Command, args []string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	params := url.Values{}
	if analyticsAgent != "" {
		params.Set("agent", analyticsAgent)
	}
	if analyticsSprint != "" {
		params.Set("sprint", analyticsSprint)
	}
	if analyticsFrom != "" {
		params.Set("from", analyticsFrom)
	}
	if analyticsTo != "" {
		params.Set("to", analyticsTo)
	}

	var endpoint string
	if analyticsCompare != "" {
		params.Set("agents", analyticsCompare)
		endpoint = "/api/analytics/agents/compare"
	} else {
		endpoint = "/api/analytics/agents"
	}

	resp, err := http.Get(cfg.ServerURL + endpoint + "?" + params.Encode())
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server error %d: %s", resp.StatusCode, body)
	}

	if analyticsCompare != "" {
		var result struct {
			Agents []analytics.AgentComparison `json:"agents"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return fmt.Errorf("decode failed: %w", err)
		}
		if analyticsFormat == "json" {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(result.Agents)
		}
		return printAgentCompareTable(result.Agents)
	}

	var result struct {
		Data []analytics.AgentStat `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode failed: %w", err)
	}

	if analyticsFormat == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result.Data)
	}
	return printAgentStatsTable(result.Data)
}

func runAnalyticsSprint(cmd *cobra.Command, args []string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	sprintID := args[0]
	resp, err := http.Get(cfg.ServerURL + "/api/analytics/sprints/" + sprintID)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server error %d: %s", resp.StatusCode, body)
	}

	var result analytics.SprintSummary
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode failed: %w", err)
	}

	fmt.Printf("Sprint: %s (%s)\n", result.SprintName, result.SprintID)
	fmt.Printf("Tasks: %d/%d completed\n", result.CompletedCount, result.TaskCount)
	fmt.Printf("Agents: %d\n", result.AgentCount)
	fmt.Printf("Runs: %d\n", result.TotalRuns)
	fmt.Printf("Cost: $%.2f\n", result.TotalCost)
	fmt.Printf("Points: %.1f\n", result.PointsCompleted)
	fmt.Printf("Points/$: %.2f\n", result.PointsPerDollar)

	return nil
}

func printCostTable(groups []analytics.CostGroup) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "GROUP\tCOST\tRUNS\tTOKENS")
	fmt.Fprintln(w, "-----\t----\t----\t------")
	for _, g := range groups {
		fmt.Fprintf(w, "%s\t$%.2f\t%d\t%d\n", g.Group, g.Cost, g.RunCount, g.Tokens)
	}
	return w.Flush()
}

func printAgentStatsTable(stats []analytics.AgentStat) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "AGENT\tRUNS\tSUCCESS\tCOST\tAVG COST\tAVG DUR")
	fmt.Fprintln(w, "-----\t----\t-------\t----\t--------\t-------")
	for _, s := range stats {
		fmt.Fprintf(w, "%s\t%d\t%.1f%%\t$%.2f\t$%.2f\t%.0fs\n",
			s.AgentName, s.RunCount, s.SuccessRate, s.TotalCost, s.AvgCost, s.AvgDuration)
	}
	return w.Flush()
}

func printAgentCompareTable(agents []analytics.AgentComparison) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "AGENT\tRUNS\tSUCCESS\tCOST\tAVG COST\tPOINTS")
	fmt.Fprintln(w, "-----\t----\t-------\t----\t--------\t------")
	for _, a := range agents {
		fmt.Fprintf(w, "%s\t%d\t%.1f%%\t$%.2f\t$%.2f\t%.1f\n",
			a.AgentName, a.RunCount, a.SuccessRate, a.TotalCost, a.AvgCost, a.PointsCompleted)
	}
	return w.Flush()
}

func loadConfig() (*struct{ ServerURL string }, error) {
	serverURL := os.Getenv("DANDORI_SERVER_URL")
	if serverURL == "" {
		serverURL = "http://localhost:8080"
	}
	serverURL = strings.TrimSuffix(serverURL, "/")
	return &struct{ ServerURL string }{ServerURL: serverURL}, nil
}
