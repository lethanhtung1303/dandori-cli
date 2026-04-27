package cmd

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/phuc-nt/dandori-cli/internal/config"
	"github.com/spf13/cobra"
)

var (
	cfgFile string
	verbose bool
	cfg     *config.Config
)

var rootCmd = &cobra.Command{
	Use:   "dandori",
	Short: "CLI outer harness for managing AI agent dev teams",
	Long: `dandori-cli wraps AI agent execution, tracks runs, integrates with
Jira/Confluence, and provides analytics for PO/PDM and QA.

It is the bridge between human project management and AI agent developers.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		var err error
		cfg, err = config.Load(cfgFile)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}

		logLevel, err := config.ParseLogLevel(cfg.LogLevel)
		if err != nil {
			return fmt.Errorf("parse log level: %w", err)
		}
		if verbose {
			logLevel = slog.LevelDebug
		}

		handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel})
		slog.SetDefault(slog.New(handler))

		return nil
	},
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: ~/.dandori/config.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
}

func Config() *config.Config {
	return cfg
}
