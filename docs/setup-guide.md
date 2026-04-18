# dandori-cli Setup Guide

## Prerequisites

- Go 1.21+
- Claude Code CLI (`claude`)
- Jira Cloud account with API token
- Confluence Cloud account (same Atlassian instance)

## Quick Start

```bash
# 1. Build
go build -o bin/dandori .

# 2. Initialize config
./bin/dandori init

# 3. Edit config
vim ~/.dandori/config.yaml
```

## Configuration

### Minimal Config (`~/.dandori/config.yaml`)

```yaml
agent:
  name: "alpha"
  type: "claude_code"

jira:
  base_url: "https://YOUR-DOMAIN.atlassian.net"
  user: "your-email@example.com"
  token: "YOUR_API_TOKEN"
  project_key: "PROJ"
  cloud: true

confluence:
  base_url: "https://YOUR-DOMAIN.atlassian.net/wiki"
  space_key: "PROJ"
  cloud: true
```

### Get Jira API Token

1. Go to https://id.atlassian.com/manage-profile/security/api-tokens
2. Click "Create API token"
3. Copy token to `config.yaml`

### Get Confluence Space Key

1. Open your Confluence space
2. Space key is in URL: `/wiki/spaces/SPACEKEY/...`

## Verify Setup

```bash
# Test Jira connection
./bin/dandori task info PROJ-1

# Test Confluence connection
./bin/dandori conf-write --task PROJ-1 --dry-run
```

## Basic Workflow

```bash
# 1. Start a task
./bin/dandori task start PROJ-123

# 2. Run agent with tracking
./bin/dandori run --task PROJ-123 -- claude "implement feature X"

# 3. Sync status back to Jira
./bin/dandori jira-sync

# 4. Write report to Confluence
./bin/dandori conf-write --task PROJ-123

# 5. View analytics
./bin/dandori dashboard
```

## Server Setup (Optional)

For team-wide analytics, run the monitoring server:

```bash
# Start PostgreSQL
docker-compose up -d postgres

# Run server
DANDORI_DB_HOST=localhost ./bin/dandori-server

# Sync local data to server
./bin/dandori sync --daemon
```

## Directory Structure

```
~/.dandori/
├── config.yaml      # Configuration
├── local.db         # SQLite database (runs, events)
└── cache/           # Confluence page cache
```

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `DANDORI_CONFIG` | Config file path | `~/.dandori/config.yaml` |
| `DANDORI_DB_PATH` | SQLite database path | `~/.dandori/local.db` |
| `DANDORI_VERBOSE` | Enable debug logging | `false` |

## Next Steps

- Read [FAQ](faq.md) for common issues
- Check `dandori --help` for all commands
- See devlog.md for implementation details
