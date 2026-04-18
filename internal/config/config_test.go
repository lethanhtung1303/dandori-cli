package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.ServerURL != "http://localhost:8080" {
		t.Errorf("expected default server URL, got %s", cfg.ServerURL)
	}
	if cfg.Agent.Type != "claude_code" {
		t.Errorf("expected agent type claude_code, got %s", cfg.Agent.Type)
	}
	if cfg.Sync.IntervalSec != 300 {
		t.Errorf("expected sync interval 300, got %d", cfg.Sync.IntervalSec)
	}
}

func TestLoadSave(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.yaml")

	cfg := DefaultConfig()
	cfg.ServerURL = "https://test.example.com"
	cfg.Jira.BaseURL = "https://jira.example.com"
	cfg.Agent.Name = "test-agent"
	cfg.Agent.Capabilities = []string{"backend", "testing"}

	if err := Save(cfg, path); err != nil {
		t.Fatalf("save config: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if loaded.ServerURL != cfg.ServerURL {
		t.Errorf("server URL mismatch: got %s, want %s", loaded.ServerURL, cfg.ServerURL)
	}
	if loaded.Jira.BaseURL != cfg.Jira.BaseURL {
		t.Errorf("jira base URL mismatch: got %s, want %s", loaded.Jira.BaseURL, cfg.Jira.BaseURL)
	}
	if loaded.Agent.Name != cfg.Agent.Name {
		t.Errorf("agent name mismatch: got %s, want %s", loaded.Agent.Name, cfg.Agent.Name)
	}
	if len(loaded.Agent.Capabilities) != 2 {
		t.Errorf("expected 2 capabilities, got %d", len(loaded.Agent.Capabilities))
	}
}

func TestEnvOverrides(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.yaml")

	cfg := DefaultConfig()
	if err := Save(cfg, path); err != nil {
		t.Fatalf("save config: %v", err)
	}

	os.Setenv("DANDORI_SERVER_URL", "https://env.example.com")
	os.Setenv("DANDORI_API_KEY", "test-key")
	os.Setenv("DANDORI_AGENT_CAPABILITIES", "go,rust")
	defer func() {
		os.Unsetenv("DANDORI_SERVER_URL")
		os.Unsetenv("DANDORI_API_KEY")
		os.Unsetenv("DANDORI_AGENT_CAPABILITIES")
	}()

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if loaded.ServerURL != "https://env.example.com" {
		t.Errorf("env override failed for server URL: got %s", loaded.ServerURL)
	}
	if loaded.APIKey != "test-key" {
		t.Errorf("env override failed for API key: got %s", loaded.APIKey)
	}
	if len(loaded.Agent.Capabilities) != 2 || loaded.Agent.Capabilities[0] != "go" {
		t.Errorf("env override failed for capabilities: got %v", loaded.Agent.Capabilities)
	}
}

func TestLoadNonExistent(t *testing.T) {
	cfg, err := Load("/nonexistent/path/config.yaml")
	if err != nil {
		t.Fatalf("expected no error for nonexistent file, got %v", err)
	}
	if cfg.ServerURL != "http://localhost:8080" {
		t.Errorf("expected default config, got %s", cfg.ServerURL)
	}
}
