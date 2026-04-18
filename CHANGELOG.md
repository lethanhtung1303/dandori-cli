# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Shell alias transparency: `dandori init` installs `claude` / `codex` aliases to `.zshrc` / `.bashrc` (vision pillar: wrapper invisibility)
- Watch daemon: `dandori watch [--once]` captures orphan runs made without the wrapper
- `internal/shellrc/` package — idempotent rc file management
- `internal/watcher/` package — session log polling
- E2E test groups K (shell alias, 5 cases) and L (watch daemon, 5 cases)
- `docs/user-guide.md` — step-by-step use cases
- `LICENSE`, `CHANGELOG.md`, `CONTRIBUTING.md`

### Fixed
- Tailer race condition: main thread no longer reads token data before the goroutine finishes
- Claude session directory detection: use path-with-dashes convention instead of SHA-256 hash
- Confluence write: parse `started_at` / `ended_at` timestamp strings into `time.Time`
- Jira client: add `CreateIssue` and `DeleteIssue` methods
- Pricing table: add `claude-opus-4-5-20251101` model

### Tests
- 308 unit tests pass across 16 packages
- 57/57 E2E tests pass (10 groups, real Jira + Confluence + Claude Code)
- Fixed bash `local out=$(...)` exit-code masking in E2E script

## [0.1.0] — 2026-04-18

Initial release — all 8 implementation phases complete.

### Added
- **Phase 01** — Foundation: Go module, Cobra CLI, SQLite, config, hash chain
- **Phase 02** — 3-layer agent wrapper (fork/exec, tailer, semantic events), cost calculation
- **Phase 03** — Jira integration: client, poller, transitions, comments
- **Phase 04** — Confluence integration: client, storage↔markdown converter, reader/writer
- **Phase 05** — Monitoring server: PostgreSQL, REST API, SSE, dashboard
- **Phase 06** — Agent assignment: 4-component scorer, engine, REST API
- **Phase 07** — Analytics: 8 query types, CSV/JSON export, CLI commands
- **Phase 08** — E2E flow: Docker Compose, mock APIs, integration tests

### Commands
- `init`, `version`, `status`
- `run`, `event`, `sync`
- `task {start,done,info}`, `jira-sync`
- `conf-write`
- `analytics {runs,agents,cost,sprint}`
- `dashboard`
- `assign {suggest,set,list}`
