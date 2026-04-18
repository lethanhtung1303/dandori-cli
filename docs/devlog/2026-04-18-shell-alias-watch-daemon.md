# 2026-04-18 | Shell Alias Transparency + Watch Daemon

Vision-aligned features per `dandori-pitch/cli-pilot-proposal.md`.

## Shell Alias Transparency

**Vision:** `dandori init` appends `alias claude='dandori run -- claude'` to user's rc file. From then on `claude "..."` is transparently wrapped. User can bypass with `\claude`.

**Implementation:**
- New package `internal/shellrc/` — pure file manipulation, no external deps
- Idempotent marker block (`# >>> dandori aliases (managed) >>>`)
- `DetectShell($SHELL)` → zsh / bash / ""
- `InstallAliases(rcFile)` returns `Installed` / `AlreadyPresent` flag
- `UninstallAliases(rcFile)` cleanly removes block preserving surrounding content

**CLI flags:**
```
dandori init --shell     # force install
dandori init --no-shell  # skip install (default-on if not --no-shell)
```

## Watch Daemon

**Vision:** Tailer runs as background daemon polling every 60s, captures runs made without the wrapper (opt-out or forgotten).

**Implementation:**
- New package `internal/watcher/` — reuses `wrapper.TokenUsage` + `wrapper.ComputeCost`
- `DiscoverProjects(root)` — lists `~/.claude/projects/*`
- `DiscoverSessions(dir)` — lists `*.jsonl` in project
- `PollOnce()` — one pass over all sessions, inserts orphan run if `session_id` not in DB
- `Run(ctx)` — loops until ctx cancelled, signal handling in CLI

**CLI:**
```
dandori watch --once                 # single pass (cron/launchd)
dandori watch                        # foreground loop, Ctrl-C to stop
dandori watch --interval 30          # custom cadence
dandori watch --root /alt/projects   # test / non-default location
```

**Skip logic:** If `runs.session_id = <sid>` already exists (from wrapper or prior poll), skip. Prevents duplicate rows.

## Testing

**Unit tests:** 14 new cases (6 shellrc + 8 watcher).

**E2E tests:** 10 new cases (K1-K5 shell, L1-L5 watch). Total: 57/57 pass.

**Highlights:**
- K2 idempotency: `grep -c` marker count stays at 2 after repeated installs
- L5 idempotency: session_id collision check prevents duplicate inserts
- L4 real token extraction: 700 tokens captured from synthetic session file

## Files

```
internal/shellrc/
├── shellrc.go         (99 lines)
└── shellrc_test.go    (120 lines)

internal/watcher/
├── watcher.go         (200 lines)
└── watcher_test.go    (180 lines)

cmd/
├── init_cmd.go        (+35 lines: --shell/--no-shell)
└── watch.go           (NEW, 95 lines)

scripts/
└── e2e-comprehensive.sh (+90 lines: Groups K, L)
```

## Vision Alignment

| Pillar | Before | After |
|--------|--------|-------|
| Tracking (wrapper) | ✅ | ✅ |
| Tracking (background) | ❌ | ✅ (watch) |
| Shell transparency | ❌ | ✅ (alias) |
| Knowledge marketplace | ❌ | ❌ (Phase 2) |
