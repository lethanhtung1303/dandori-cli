package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/phuc-nt/dandori-cli/internal/config"
	"github.com/phuc-nt/dandori-cli/internal/db"
	"github.com/phuc-nt/dandori-cli/internal/event"
	"github.com/phuc-nt/dandori-cli/internal/model"
	"github.com/spf13/cobra"
)

var eventCmd = &cobra.Command{
	Use:   "event",
	Short: "Record a Layer 3 event from agent execution",
	Long: `Records semantic events emitted by the agent during execution.
This command is called BY the agent (via Claude Code's bash tool).

Examples:
  dandori event --run abc123 --type decision --data '{"rationale":"chose pagination"}'
  dandori event --run abc123 --type files_touched --data '{"files":["src/auth.ts"]}'`,
	RunE: runEvent,
}

var (
	eventRunID string
	eventType  string
	eventData  string
)

func init() {
	eventCmd.Flags().StringVar(&eventRunID, "run", "", "Run ID to link event to (required)")
	eventCmd.Flags().StringVar(&eventType, "type", "", "Event type (decision, file_change, task_link, custom)")
	eventCmd.Flags().StringVar(&eventData, "data", "{}", "JSON payload")
	eventCmd.MarkFlagRequired("run")
	eventCmd.MarkFlagRequired("type")
	rootCmd.AddCommand(eventCmd)
}

func runEvent(cmd *cobra.Command, args []string) error {
	dbPath, err := config.DBPath()
	if err != nil {
		return err
	}

	localDB, err := db.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer localDB.Close()

	var exists int
	err = localDB.QueryRow(`SELECT COUNT(*) FROM runs WHERE id = ?`, eventRunID).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check run: %w", err)
	}
	if exists == 0 {
		return fmt.Errorf("run %s not found", eventRunID)
	}

	var data any
	if err := json.Unmarshal([]byte(eventData), &data); err != nil {
		return fmt.Errorf("invalid JSON data: %w", err)
	}

	recorder := event.NewRecorder(localDB)
	if err := recorder.RecordEvent(eventRunID, model.LayerSkill, eventType, data); err != nil {
		return fmt.Errorf("record event: %w", err)
	}

	fmt.Printf("Event recorded: run=%s type=%s\n", eventRunID, eventType)
	return nil
}
