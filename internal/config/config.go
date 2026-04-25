package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	ServerURL  string           `yaml:"server_url"`
	APIKey     string           `yaml:"api_key"`
	Jira       JiraConfig       `yaml:"jira"`
	Confluence ConfluenceConfig `yaml:"confluence"`
	Agent      AgentConfig      `yaml:"agent"`
	Project    ProjectConfig    `yaml:"project"`
	Sync       SyncConfig       `yaml:"sync"`
	Quality    QualityConfig    `yaml:"quality"`
	Wrapper    WrapperConfig    `yaml:"wrapper"`
}

type WrapperConfig struct {
	PostExitTimeout string `yaml:"post_exit_timeout"`
}

type QualityConfig struct {
	Enabled     bool   `yaml:"enabled"`
	LintCommand string `yaml:"lint_command"`
	TestCommand string `yaml:"test_command"`
	Timeout     string `yaml:"timeout"`
}

type JiraConfig struct {
	BaseURL         string            `yaml:"base_url"`
	User            string            `yaml:"user"`
	Token           string            `yaml:"token"`
	BoardID         int               `yaml:"board_id"`
	PollIntervalSec int               `yaml:"poll_interval_sec"`
	AgentFieldID    string            `yaml:"agent_field_id"`
	StatusMapping   map[string]string `yaml:"status_mapping"`
	Cloud           bool              `yaml:"cloud"`
}

type ConfluenceConfig struct {
	BaseURL             string `yaml:"base_url"`
	User                string `yaml:"user"`
	Token               string `yaml:"token"`
	SpaceKey            string `yaml:"space_key"`
	ReportsParentPageID string `yaml:"reports_parent_page_id"`
	AutoPost            bool   `yaml:"auto_post"`
	CacheTTLMin         int    `yaml:"cache_ttl_min"`
	Cloud               bool   `yaml:"cloud"`
}

type AgentConfig struct {
	Type         string   `yaml:"type"`
	Name         string   `yaml:"name"`
	Capabilities []string `yaml:"capabilities"`
}

type ProjectConfig struct {
	Key  string `yaml:"key"`
	Team string `yaml:"team"`
}

type SyncConfig struct {
	IntervalSec int `yaml:"interval_sec"`
	BatchSize   int `yaml:"batch_size"`
}

func DefaultConfig() *Config {
	return &Config{
		ServerURL: "http://localhost:8080",
		Agent: AgentConfig{
			Type: "claude_code",
			Name: "default",
		},
		Sync: SyncConfig{
			IntervalSec: 300,
			BatchSize:   100,
		},
		Quality: QualityConfig{
			Enabled:     true,
			LintCommand: "golangci-lint run --json --out-format json 2>/dev/null || true",
			TestCommand: "go test -json -count=1 ./... 2>&1 || true",
			Timeout:     "30s",
		},
		Wrapper: WrapperConfig{
			PostExitTimeout: "10s",
		},
	}
}

func ConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}
	return filepath.Join(home, ".dandori"), nil
}

func ConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

func DBPath() (string, error) {
	if env := os.Getenv("DANDORI_DB"); env != "" {
		return env, nil
	}
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	// Honor `~/.dandori/active_db` pointer file if present — used by
	// `dandori demo --use` to swap in demo.db without editing config.
	pointer := filepath.Join(dir, "active_db")
	if data, err := os.ReadFile(pointer); err == nil {
		p := strings.TrimSpace(string(data))
		if p != "" {
			return p, nil
		}
	}
	return filepath.Join(dir, "local.db"), nil
}

func Load(path string) (*Config, error) {
	cfg := DefaultConfig()

	if path == "" {
		var err error
		path, err = ConfigPath()
		if err != nil {
			return nil, err
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			applyEnvOverrides(cfg)
			return cfg, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	applyEnvOverrides(cfg)
	return cfg, nil
}

func Save(cfg *Config, path string) error {
	if path == "" {
		var err error
		path, err = ConfigPath()
		if err != nil {
			return err
		}
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	return nil
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("DANDORI_SERVER_URL"); v != "" {
		cfg.ServerURL = v
	}
	if v := os.Getenv("DANDORI_API_KEY"); v != "" {
		cfg.APIKey = v
	}
	if v := os.Getenv("DANDORI_JIRA_BASE_URL"); v != "" {
		cfg.Jira.BaseURL = v
	}
	if v := os.Getenv("DANDORI_JIRA_USER"); v != "" {
		cfg.Jira.User = v
	}
	if v := os.Getenv("DANDORI_JIRA_TOKEN"); v != "" {
		cfg.Jira.Token = v
	}
	if v := os.Getenv("DANDORI_CONFLUENCE_BASE_URL"); v != "" {
		cfg.Confluence.BaseURL = v
	}
	if v := os.Getenv("DANDORI_CONFLUENCE_SPACE_KEY"); v != "" {
		cfg.Confluence.SpaceKey = v
	}
	if v := os.Getenv("DANDORI_CONFLUENCE_REPORTS_PARENT_PAGE_ID"); v != "" {
		cfg.Confluence.ReportsParentPageID = v
	}
	if v := os.Getenv("DANDORI_AGENT_TYPE"); v != "" {
		cfg.Agent.Type = v
	}
	if v := os.Getenv("DANDORI_AGENT_NAME"); v != "" {
		cfg.Agent.Name = v
	}
	if v := os.Getenv("DANDORI_AGENT_CAPABILITIES"); v != "" {
		cfg.Agent.Capabilities = strings.Split(v, ",")
	}
	if v := os.Getenv("DANDORI_PROJECT_KEY"); v != "" {
		cfg.Project.Key = v
	}
	if v := os.Getenv("DANDORI_PROJECT_TEAM"); v != "" {
		cfg.Project.Team = v
	}
}
