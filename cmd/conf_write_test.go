package cmd

import (
	"strings"
	"testing"
)

func TestConfWriteCommandExists(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "conf-write" {
			found = true
			break
		}
	}
	if !found {
		t.Error("conf-write command should be registered")
	}
}

func TestConfWriteFlags(t *testing.T) {
	cmd := confWriteCmd

	// Check required flags exist
	runFlag := cmd.Flag("run")
	if runFlag == nil {
		t.Error("--run flag should exist")
	}

	taskFlag := cmd.Flag("task")
	if taskFlag == nil {
		t.Error("--task flag should exist")
	}

	dryRunFlag := cmd.Flag("dry-run")
	if dryRunFlag == nil {
		t.Error("--dry-run flag should exist")
	}
}

func TestConfWriteUsage(t *testing.T) {
	usage := confWriteCmd.UsageString()

	if !strings.Contains(usage, "--run") {
		t.Error("usage should mention --run flag")
	}
	if !strings.Contains(usage, "--task") {
		t.Error("usage should mention --task flag")
	}
}

func TestConfWriteRequiresRunOrTask(t *testing.T) {
	// Reset flags
	confWriteRunID = ""
	confWriteTaskKey = ""

	err := runConfWrite(confWriteCmd, []string{})
	if err == nil {
		t.Error("should require either --run or --task")
	}
	if !strings.Contains(err.Error(), "run") && !strings.Contains(err.Error(), "task") {
		t.Errorf("error should mention run or task requirement: %v", err)
	}
}
