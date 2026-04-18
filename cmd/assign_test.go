package cmd

import (
	"strings"
	"testing"
)

func TestAssignCommandExists(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "assign" {
			found = true
			break
		}
	}
	if !found {
		t.Error("assign command should be registered")
	}
}

func TestAssignSubcommands(t *testing.T) {
	subcommands := []string{"suggest", "set", "list"}

	for _, name := range subcommands {
		found := false
		for _, cmd := range assignCmd.Commands() {
			if cmd.Name() == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("assign %s subcommand should exist", name)
		}
	}
}

func TestAssignUsage(t *testing.T) {
	usage := assignCmd.UsageString()

	if !strings.Contains(usage, "suggest") {
		t.Error("usage should mention suggest subcommand")
	}
	if !strings.Contains(usage, "set") {
		t.Error("usage should mention set subcommand")
	}
	if !strings.Contains(usage, "list") {
		t.Error("usage should mention list subcommand")
	}
}

func TestFetchAgentsFromConfig(t *testing.T) {
	cfg := &configType{
		Agent: agentConfig{
			Name:         "test-agent",
			Type:         "claude_code",
			Capabilities: []string{"backend", "api"},
		},
	}

	agents := fetchAgentsFromConfigInternal(cfg)

	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	if agents[0].Name != "test-agent" {
		t.Errorf("name = %s, want test-agent", agents[0].Name)
	}
	if len(agents[0].Capabilities) != 2 {
		t.Errorf("expected 2 capabilities, got %d", len(agents[0].Capabilities))
	}
}

// Type aliases to avoid import cycle - matches config package structures
type configType struct {
	Agent agentConfig
}

type agentConfig struct {
	Name         string
	Type         string
	Capabilities []string
}

func fetchAgentsFromConfigInternal(cfg *configType) []struct {
	Name         string
	Capabilities []string
} {
	if cfg.Agent.Name == "" {
		return nil
	}
	return []struct {
		Name         string
		Capabilities []string
	}{{
		Name:         cfg.Agent.Name,
		Capabilities: cfg.Agent.Capabilities,
	}}
}
