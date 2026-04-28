//go:build !windows

package quality

import (
	"context"
	"os/exec"
	"syscall"
	"time"
)

// spawnCollectorCmd creates an exec.Cmd for a shell command with process-group
// isolation so that context cancellation kills the entire child tree, not just
// the "sh" parent. Without Setpgid the grandchild (e.g. "go test") survives sh's
// death, keeps the stdout pipe write-end open, and cmd.Output() blocks forever.
// WaitDelay caps the pipe-drain wait so a rogue grandchild can't cause infinite hang.
func spawnCollectorCmd(ctx context.Context, shellCmd string, waitDelay time.Duration) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "sh", "-c", shellCmd)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = waitDelay
	return cmd
}
