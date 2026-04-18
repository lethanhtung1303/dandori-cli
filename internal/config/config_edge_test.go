package config

import (
	"os"
	"path/filepath"
	"testing"
)

// Scenario 1: Empty config file
func TestLoadEmptyConfigFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.yaml")
	os.WriteFile(path, []byte(""), 0644)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("empty config should use defaults: %v", err)
	}
	if cfg.ServerURL != "http://localhost:8080" {
		t.Error("should use default server URL")
	}
}

// Scenario 2: Malformed YAML syntax
func TestLoadMalformedYAML(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.yaml")
	os.WriteFile(path, []byte("server_url: [invalid yaml"), 0644)

	_, err := Load(path)
	if err == nil {
		t.Error("malformed YAML should return error")
	}
}

// Scenario 3: YAML with unknown fields
func TestLoadUnknownFields(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.yaml")
	os.WriteFile(path, []byte("unknown_field: value\nserver_url: http://test.com"), 0644)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unknown fields should be ignored: %v", err)
	}
	if cfg.ServerURL != "http://test.com" {
		t.Error("known fields should be parsed")
	}
}

// Scenario 4: Config path with unicode chars
func TestLoadUnicodePath(t *testing.T) {
	tmpDir := t.TempDir()
	unicodeDir := filepath.Join(tmpDir, "配置文件")
	os.MkdirAll(unicodeDir, 0755)
	path := filepath.Join(unicodeDir, "config.yaml")
	os.WriteFile(path, []byte("server_url: http://unicode.com"), 0644)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unicode path should work: %v", err)
	}
	if cfg.ServerURL != "http://unicode.com" {
		t.Error("should load from unicode path")
	}
}

// Scenario 5: Config dir doesn't exist on save
func TestSaveCreatesDir(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "new", "nested", "config.yaml")

	cfg := DefaultConfig()
	err := Save(cfg, path)
	if err != nil {
		t.Fatalf("save should create dirs: %v", err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("config file should exist")
	}
}

// Scenario 7: Config file permission denied
func TestLoadPermissionDenied(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("skipping permission test as root")
	}

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.yaml")
	os.WriteFile(path, []byte("server_url: http://test.com"), 0000)
	defer os.Chmod(path, 0644)

	_, err := Load(path)
	if err == nil {
		t.Error("should return permission error")
	}
}

// Scenario 8: Env override with empty string
func TestEnvOverrideEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.yaml")
	os.WriteFile(path, []byte("server_url: http://yaml.com"), 0644)

	os.Setenv("DANDORI_SERVER_URL", "")
	defer os.Unsetenv("DANDORI_SERVER_URL")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if cfg.ServerURL != "http://yaml.com" {
		t.Error("empty env should not override YAML value")
	}
}

// Additional: Deep nested config
func TestLoadDeepNestedConfig(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.yaml")
	yaml := `
jira:
  base_url: https://jira.example.com
  status_mapping:
    running: In Progress
    done: Done
agent:
  capabilities:
    - backend
    - testing
`
	os.WriteFile(path, []byte(yaml), 0644)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if cfg.Jira.BaseURL != "https://jira.example.com" {
		t.Error("nested jira config failed")
	}
	if len(cfg.Agent.Capabilities) != 2 {
		t.Error("array parsing failed")
	}
}
