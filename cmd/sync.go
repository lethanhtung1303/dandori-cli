package cmd

import (
	"fmt"

	"github.com/phuc-nt/dandori-cli/internal/config"
	"github.com/phuc-nt/dandori-cli/internal/db"
	"github.com/spf13/cobra"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Upload pending runs and events to monitoring server",
	Long: `Syncs local runs and events to the central monitoring server.
Events are batched and uploaded in order. Successfully synced items
are marked as synced=1 in local.db.

The sync command will be fully implemented in Phase 05 (Monitoring Server).`,
	RunE: runSync,
}

func init() {
	rootCmd.AddCommand(syncCmd)
}

func runSync(cmd *cobra.Command, args []string) error {
	cfg := Config()
	if cfg == nil || cfg.ServerURL == "" {
		return fmt.Errorf("server_url not configured. Run 'dandori init' or edit ~/.dandori/config.yaml")
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

	var unsyncedRuns, unsyncedEvents int
	localDB.QueryRow(`SELECT COUNT(*) FROM runs WHERE synced = 0`).Scan(&unsyncedRuns)
	localDB.QueryRow(`SELECT COUNT(*) FROM events WHERE synced = 0`).Scan(&unsyncedEvents)

	if unsyncedRuns == 0 && unsyncedEvents == 0 {
		fmt.Println("Nothing to sync.")
		return nil
	}

	fmt.Printf("Pending sync: %d runs, %d events\n", unsyncedRuns, unsyncedEvents)
	fmt.Printf("Server: %s\n", cfg.ServerURL)
	fmt.Println("\n[Sync will be implemented in Phase 05 - Monitoring Server]")

	return nil
}
