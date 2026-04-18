# Contributing to dandori-cli

Thanks for your interest. This project is in early-stage development — contributions welcome.

## Development Setup

```bash
git clone https://github.com/phuc-nt/dandori-cli.git
cd dandori-cli
go mod download
make build
```

## Running Tests

```bash
make test                                          # unit tests (308 cases)
go test ./internal/jira/... -tags=integration -v   # requires Jira credentials
./scripts/e2e-comprehensive.sh                     # full E2E (requires real Jira + Confluence + Claude)
```

E2E tests require environment variables:
```bash
export DANDORI_JIRA_URL="https://YOUR-DOMAIN.atlassian.net"
export DANDORI_JIRA_USER="you@example.com"
export DANDORI_JIRA_TOKEN="atlassian-api-token"
```

## Code Standards

- Go conventions: `gofmt`, standard project layout
- File naming: `snake_case.go` (Go convention)
- Package naming: short, lowercase, single-word when possible
- Error handling: `fmt.Errorf("context: %w", err)`, never ignore
- Testing: table-driven tests preferred
- No CGO: pure Go only (enables cross-compile)
- Logging: `slog` stdlib, structured output

## Commit Messages

Conventional Commits format:
```
type: short description

optional longer body
```

Types: `feat`, `fix`, `docs`, `refactor`, `test`, `chore`

Example:
```
feat: shell alias transparency + watch daemon (TDD)
```

## Pull Request Checklist

- [ ] `make test` passes
- [ ] New features have tests (unit + E2E if user-facing)
- [ ] Commits follow conventional format
- [ ] Documentation updated (`docs/`, `README.md`, `CHANGELOG.md` if relevant)
- [ ] No secrets in commits (check `.env`, `config.yaml`, API tokens)

## Reporting Issues

Include:
- `dandori version` output
- OS and Go version
- Steps to reproduce
- Expected vs actual behavior
- Relevant logs (`~/.dandori/*.log` if present)

## Architecture Decisions

Design principles:
1. **Wrapper is non-negotiable** — Layer 1 captures every run even if Layers 2–3 fail
2. **CLI-heavy** — server is optional; SQLite is enough for single-workstation
3. **Jira IS the task board** — don't duplicate
4. **Confluence IS the knowledge store** — don't duplicate
5. **Cloud-first** — Atlassian Cloud API first, Data Center later
6. **Single binary** — zero runtime dependencies
7. **Offline-capable** — local SQLite works without network

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
