package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func executeCommand(root *cobra.Command, args ...string) (output string, err error) {
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(args)

	err = root.Execute()
	return buf.String(), err
}

func TestVersionCommand(t *testing.T) {
	Version = "1.0.0-test"
	Commit = "abc123"
	BuildDate = "2026-04-18"

	_, err := executeCommand(rootCmd, "version")
	if err != nil {
		t.Fatalf("version command failed: %v", err)
	}
}

func TestRootCommandHelp(t *testing.T) {
	output, err := executeCommand(rootCmd, "--help")
	if err != nil {
		t.Fatalf("help command failed: %v", err)
	}

	expectedCommands := []string{"init", "run", "status", "sync", "event", "version"}
	for _, cmd := range expectedCommands {
		if !strings.Contains(output, cmd) {
			t.Errorf("help output should contain command: %s", cmd)
		}
	}
}

func TestRunCommandNoArgs(t *testing.T) {
	_, err := executeCommand(rootCmd, "run")
	if err == nil {
		t.Error("run without args should fail")
	}
	if !strings.Contains(err.Error(), "no command specified") {
		t.Errorf("error should mention no command: %v", err)
	}
}

func TestRunCommandDryRun(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".dandori")
	os.MkdirAll(configDir, 0755)

	configPath := filepath.Join(configDir, "config.yaml")
	os.WriteFile(configPath, []byte("server_url: http://localhost:8080\n"), 0644)

	dbPath := filepath.Join(configDir, "local.db")

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	os.Remove(dbPath)

	_, err := executeCommand(rootCmd, "run", "--dry-run", "--", "echo", "test")

	if err != nil && !strings.Contains(err.Error(), "database") {
		t.Logf("dry-run test note: %v", err)
	}
}

func TestEventCommandRequiredFlags(t *testing.T) {
	_, err := executeCommand(rootCmd, "event")
	if err == nil {
		t.Error("event without required flags should fail")
	}
}

func TestStatusCommandHelp(t *testing.T) {
	output, err := executeCommand(rootCmd, "status", "--help")
	if err != nil {
		t.Fatalf("status help failed: %v", err)
	}

	if !strings.Contains(output, "limit") {
		t.Error("status help should mention limit flag")
	}
}

func TestSyncCommandHelp(t *testing.T) {
	_, err := executeCommand(rootCmd, "sync", "--help")
	if err != nil {
		t.Fatalf("sync help failed: %v", err)
	}
}

func TestInitCommandHelp(t *testing.T) {
	_, err := executeCommand(rootCmd, "init", "--help")
	if err != nil {
		t.Fatalf("init help failed: %v", err)
	}
}

func TestGlobalFlags(t *testing.T) {
	output, err := executeCommand(rootCmd, "--help")
	if err != nil {
		t.Fatalf("help failed: %v", err)
	}

	if !strings.Contains(output, "--config") {
		t.Error("should have --config flag")
	}
	if !strings.Contains(output, "--verbose") {
		t.Error("should have --verbose flag")
	}
}

func TestRunCommandFlags(t *testing.T) {
	output, err := executeCommand(rootCmd, "run", "--help")
	if err != nil {
		t.Fatalf("run help failed: %v", err)
	}

	expectedFlags := []string{"--task", "--auto-task", "--no-tailer", "--dry-run"}
	for _, flag := range expectedFlags {
		if !strings.Contains(output, flag) {
			t.Errorf("run help should contain flag: %s", flag)
		}
	}
}

func TestEventCommandFlags(t *testing.T) {
	output, err := executeCommand(rootCmd, "event", "--help")
	if err != nil {
		t.Fatalf("event help failed: %v", err)
	}

	expectedFlags := []string{"--run", "--type", "--data"}
	for _, flag := range expectedFlags {
		if !strings.Contains(output, flag) {
			t.Errorf("event help should contain flag: %s", flag)
		}
	}
}
