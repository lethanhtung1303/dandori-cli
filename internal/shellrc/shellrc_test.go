package shellrc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectShell(t *testing.T) {
	tests := []struct {
		shellEnv string
		want     string
	}{
		{"/bin/zsh", "zsh"},
		{"/usr/bin/bash", "bash"},
		{"/opt/homebrew/bin/zsh", "zsh"},
		{"/bin/fish", ""},
		{"", ""},
	}

	for _, tt := range tests {
		got := DetectShell(tt.shellEnv)
		if got != tt.want {
			t.Errorf("DetectShell(%q) = %q, want %q", tt.shellEnv, got, tt.want)
		}
	}
}

func TestRCFilePath(t *testing.T) {
	tests := []struct {
		shell    string
		wantFile string
	}{
		{"zsh", ".zshrc"},
		{"bash", ".bashrc"},
		{"fish", ""},
	}

	for _, tt := range tests {
		got := RCFileName(tt.shell)
		if got != tt.wantFile {
			t.Errorf("RCFileName(%q) = %q, want %q", tt.shell, got, tt.wantFile)
		}
	}
}

func TestInstallAliases_FreshFile(t *testing.T) {
	tmpDir := t.TempDir()
	rcFile := filepath.Join(tmpDir, ".zshrc")
	os.WriteFile(rcFile, []byte("# existing content\nexport PATH=/bin\n"), 0644)

	result, err := InstallAliases(rcFile)
	if err != nil {
		t.Fatalf("InstallAliases: %v", err)
	}
	if !result.Installed {
		t.Error("expected Installed=true")
	}

	content, _ := os.ReadFile(rcFile)
	s := string(content)
	if !strings.Contains(s, StartMarker) {
		t.Error("missing start marker")
	}
	if !strings.Contains(s, EndMarker) {
		t.Error("missing end marker")
	}
	if !strings.Contains(s, "alias claude=") {
		t.Error("missing claude alias")
	}
	if !strings.Contains(s, "# existing content") {
		t.Error("original content removed")
	}
}

func TestInstallAliases_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()
	rcFile := filepath.Join(tmpDir, ".zshrc")
	os.WriteFile(rcFile, []byte(""), 0644)

	// First install
	if _, err := InstallAliases(rcFile); err != nil {
		t.Fatalf("first install: %v", err)
	}

	// Second install should be no-op
	result, err := InstallAliases(rcFile)
	if err != nil {
		t.Fatalf("second install: %v", err)
	}
	if result.Installed {
		t.Error("second install should report not installed")
	}
	if !result.AlreadyPresent {
		t.Error("expected AlreadyPresent=true")
	}

	// Verify no duplication
	content, _ := os.ReadFile(rcFile)
	count := strings.Count(string(content), StartMarker)
	if count != 1 {
		t.Errorf("marker count = %d, want 1", count)
	}
}

func TestInstallAliases_CreateIfMissing(t *testing.T) {
	tmpDir := t.TempDir()
	rcFile := filepath.Join(tmpDir, ".zshrc")

	result, err := InstallAliases(rcFile)
	if err != nil {
		t.Fatalf("InstallAliases: %v", err)
	}
	if !result.Installed {
		t.Error("expected Installed=true")
	}

	if _, err := os.Stat(rcFile); os.IsNotExist(err) {
		t.Error("RC file was not created")
	}
}

func TestUninstallAliases(t *testing.T) {
	tmpDir := t.TempDir()
	rcFile := filepath.Join(tmpDir, ".zshrc")
	original := "# original\nexport PATH=/bin\n"
	os.WriteFile(rcFile, []byte(original), 0644)

	// Install
	if _, err := InstallAliases(rcFile); err != nil {
		t.Fatalf("install: %v", err)
	}

	// Uninstall
	if err := UninstallAliases(rcFile); err != nil {
		t.Fatalf("uninstall: %v", err)
	}

	content, _ := os.ReadFile(rcFile)
	s := string(content)
	if strings.Contains(s, StartMarker) {
		t.Error("marker not removed")
	}
	if !strings.Contains(s, "# original") {
		t.Error("original content destroyed")
	}
}
