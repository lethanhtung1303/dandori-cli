package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/phuc-nt/dandori-cli/internal/config"
	"github.com/phuc-nt/dandori-cli/internal/db"
	"github.com/phuc-nt/dandori-cli/internal/watcher"
	"github.com/spf13/cobra"
)

var (
	watchOnce     bool
	watchInterval int
	watchRoot     string
)

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Watch Claude session logs and capture orphan runs",
	Long: `Poll Claude session log directories. For any session that is not already
tracked by the dandori wrapper, insert an orphan run row with tokens and cost.

Run in foreground with Ctrl-C to stop, or use --once for a single pass (useful in
cron / launchd / systemd timers).`,
	RunE: runWatch,
}

func init() {
	watchCmd.Flags().BoolVar(&watchOnce, "once", false, "Poll once and exit (for cron / manual runs)")
	watchCmd.Flags().IntVar(&watchInterval, "interval", 60, "Polling interval in seconds")
	watchCmd.Flags().StringVar(&watchRoot, "root", "", "Override Claude projects root (default: ~/.claude/projects)")
	rootCmd.AddCommand(watchCmd)
}

func runWatch(cmd *cobra.Command, args []string) error {
	dbPath, err := config.DBPath()
	if err != nil {
		return err
	}
	localDB, err := db.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer localDB.Close()

	root := watchRoot
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		root = filepath.Join(home, ".claude", "projects")
	}

	w := watcher.New(watcher.Config{
		DB:                 localDB,
		ClaudeProjectsRoot: root,
		Interval:           time.Duration(watchInterval) * time.Second,
	})

	if watchOnce {
		if err := w.PollOnce(); err != nil {
			return fmt.Errorf("poll: %w", err)
		}
		fmt.Println("watch: single poll complete")
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		cancel()
	}()

	fmt.Printf("watch: polling %s every %ds (Ctrl-C to stop)\n", root, watchInterval)
	if err := w.Run(ctx); err != nil && err != context.Canceled {
		return err
	}
	fmt.Println("watch: stopped")
	return nil
}
