package cmd

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Print(formatVersion(Version, Commit))
		fmt.Printf("  built:      %s\n", BuildDate)
		fmt.Printf("  go version: %s\n", runtime.Version())
		fmt.Printf("  platform:   %s/%s\n", runtime.GOOS, runtime.GOARCH)
	},
}

func formatVersion(v, sha string) string {
	return fmt.Sprintf("dandori-cli %s\n  commit:     %s\n", v, sha)
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
