package cmd

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/phuc-nt/dandori-cli/internal/config"
	"github.com/phuc-nt/dandori-cli/internal/db"
	"github.com/phuc-nt/dandori-cli/internal/util"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize dandori configuration and database",
	Long: `Creates ~/.dandori/ directory with config.yaml template and local SQLite database.
Prompts for Jira/Confluence URLs and agent configuration.`,
	RunE: runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	configDir, err := config.ConfigDir()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	slog.Info("created config directory", "path", configDir)

	configPath, err := config.ConfigPath()
	if err != nil {
		return err
	}

	cfg := config.DefaultConfig()

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		reader := bufio.NewReader(os.Stdin)

		fmt.Print("Server URL [http://localhost:8080]: ")
		if serverURL, _ := reader.ReadString('\n'); strings.TrimSpace(serverURL) != "" {
			cfg.ServerURL = strings.TrimSpace(serverURL)
		}

		fmt.Print("Jira Base URL (optional): ")
		if jiraURL, _ := reader.ReadString('\n'); strings.TrimSpace(jiraURL) != "" {
			cfg.Jira.BaseURL = strings.TrimSpace(jiraURL)
		}

		fmt.Print("Agent Name [default]: ")
		if agentName, _ := reader.ReadString('\n'); strings.TrimSpace(agentName) != "" {
			cfg.Agent.Name = strings.TrimSpace(agentName)
		}

		fmt.Print("Project Key (Jira project): ")
		if projectKey, _ := reader.ReadString('\n'); strings.TrimSpace(projectKey) != "" {
			cfg.Project.Key = strings.TrimSpace(projectKey)
		}

		if err := config.Save(cfg, configPath); err != nil {
			return fmt.Errorf("save config: %w", err)
		}
		slog.Info("created config file", "path", configPath)
	} else {
		slog.Info("config file already exists", "path", configPath)
	}

	dbPath, err := config.DBPath()
	if err != nil {
		return err
	}

	localDB, err := db.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer localDB.Close()

	if err := localDB.Migrate(); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}
	slog.Info("initialized database", "path", dbPath)

	wsID := util.GenerateWorkstationID()
	fmt.Printf("\nInitialization complete!\n")
	fmt.Printf("  Config:      %s\n", configPath)
	fmt.Printf("  Database:    %s\n", dbPath)
	fmt.Printf("  Workstation: %s\n", wsID)
	fmt.Printf("\nNext steps:\n")
	fmt.Printf("  1. Edit %s to add Jira/Confluence credentials\n", configPath)
	fmt.Printf("  2. Run 'dandori run -- claude ...' to start tracking agent runs\n")

	return nil
}
