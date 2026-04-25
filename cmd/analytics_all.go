package cmd

import (
	"fmt"
	"time"

	"github.com/phuc-nt/dandori-cli/internal/analytics"
	"github.com/spf13/cobra"
)

var (
	analyticsAllSinceDays int
)

var analyticsAllCmd = &cobra.Command{
	Use:   "all",
	Short: "One-shot snapshot: cost + leaderboard + quality + alerts",
	Long: `Show the full blog-style dashboard in a single command.

Example:
  dandori analytics all --since 7
  dandori analytics all --format json`,
	RunE: runAnalyticsAll,
}

func init() {
	analyticsCmd.AddCommand(analyticsAllCmd)
	analyticsAllCmd.Flags().IntVar(&analyticsAllSinceDays, "since", 30, "Window in days")
	analyticsAllCmd.Flags().StringVar(&analyticsFormat, "format", "table", "Output format: table, json")
}

func runAnalyticsAll(cmd *cobra.Command, args []string) error {
	store, err := getLocalDB()
	if err != nil {
		return err
	}
	defer store.Close()

	win := analytics.Window{Since: time.Duration(analyticsAllSinceDays) * 24 * time.Hour}
	snap, err := analytics.BuildSnapshot(store, win, analytics.DefaultThresholds())
	if err != nil {
		return fmt.Errorf("build snapshot: %w", err)
	}

	if analyticsFormat == "json" {
		fmt.Println(analytics.FormatJSON(snap))
		return nil
	}

	fmt.Print(analytics.FormatTable(snap))
	return nil
}
