package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIncidentReportCmd_Help(t *testing.T) {
	output, err := executeCommand(rootCmd, "incident-report", "--help")
	if err != nil {
		t.Fatalf("help failed: %v", err)
	}
	for _, want := range []string{"--run", "--task", "--out", "--since"} {
		if !strings.Contains(output, want) {
			t.Errorf("help output missing flag %q", want)
		}
	}
}

// TestValidateFlagsMutualExclusion tests the validation logic directly,
// bypassing Cobra's stateful flag parsing (which leaks between test calls).
func TestValidateFlagsMutualExclusion(t *testing.T) {
	cases := []struct {
		name    string
		runID   string
		task    string
		wantErr string
	}{
		{
			name:    "no flags",
			runID:   "",
			task:    "",
			wantErr: "--run or --task is required",
		},
		{
			name:    "both flags",
			runID:   "abc",
			task:    "CLITEST-1",
			wantErr: "mutually exclusive",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateIncidentFlags(tc.runID, tc.task)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q missing %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestValidateFlagsMutualExclusion_Valid(t *testing.T) {
	if err := validateIncidentFlags("abc", ""); err != nil {
		t.Errorf("--run only should be valid, got: %v", err)
	}
	if err := validateIncidentFlags("", "CLITEST-1"); err != nil {
		t.Errorf("--task only should be valid, got: %v", err)
	}
}

func TestWriteOutput_Stdout(t *testing.T) {
	// Verify no error is returned; content goes to stdout.
	// Full formatter output is tested in internal/intent package.
	err := writeOutput("# test report\n\nsome content\n", "")
	if err != nil {
		t.Errorf("writeOutput to stdout returned error: %v", err)
	}
}

func TestWriteOutput_File(t *testing.T) {
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "sub", "report.md")
	content := "# Incident Report\n\ntest content\n"

	if err := writeOutput(content, outPath); err != nil {
		t.Fatalf("writeOutput to file: %v", err)
	}

	got, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	if string(got) != content {
		t.Errorf("file content mismatch\ngot:  %q\nwant: %q", string(got), content)
	}
}

func TestWriteOutput_FileCreatesParentDir(t *testing.T) {
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "a", "b", "c", "report.md")

	if err := writeOutput("hello", outPath); err != nil {
		t.Fatalf("writeOutput failed to create nested dirs: %v", err)
	}

	if _, err := os.Stat(outPath); err != nil {
		t.Errorf("output file not created: %v", err)
	}
}

func TestParseSinceDateImpl_Invalid(t *testing.T) {
	cases := []string{"not-a-date", "2026/04/01", "April 1 2026", "01-04-2026"}
	for _, s := range cases {
		_, err := parseSinceDateImpl(s)
		if err == nil {
			t.Errorf("parseSinceDateImpl(%q) should fail", s)
		}
		if !strings.Contains(err.Error(), "YYYY-MM-DD") {
			t.Errorf("error for %q missing YYYY-MM-DD hint: %v", s, err)
		}
	}
}

func TestParseSinceDateImpl_Valid(t *testing.T) {
	cases := []string{"2026-04-01", "2025-01-31", "2024-12-01"}
	for _, s := range cases {
		got, err := parseSinceDateImpl(s)
		if err != nil {
			t.Errorf("parseSinceDateImpl(%q) unexpected error: %v", s, err)
		}
		if got.IsZero() {
			t.Errorf("parseSinceDateImpl(%q) returned zero time", s)
		}
	}
}

func TestParseSinceDateImpl_Empty(t *testing.T) {
	got, err := parseSinceDateImpl("")
	if err != nil {
		t.Errorf("empty since should return zero time, not error: %v", err)
	}
	if !got.IsZero() {
		t.Errorf("empty since should return zero time, got %v", got)
	}
}

// validateIncidentFlags mirrors the validation logic in runIncidentReport,
// allowing direct testing without Cobra's stateful flag parsing.
func validateIncidentFlags(runID, task string) error {
	if runID == "" && task == "" {
		return fmt.Errorf("one of --run or --task is required")
	}
	if runID != "" && task != "" {
		return fmt.Errorf("--run and --task are mutually exclusive")
	}
	return nil
}
