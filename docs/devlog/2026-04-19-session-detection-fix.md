# 2026-04-19 — Session Detection Bug Fix

## Summary

Fixed critical bug where token/cost capture failed on macOS due to symlink resolution issue.

## Problem

Session log detection failed when working directory used `/tmp` symlink:
- Wrapper gets cwd: `/tmp/project`
- Claude stores session: `~/.claude/projects/-private-tmp-project`
- Wrapper looks for: `~/.claude/projects/-tmp-project` (doesn't exist)

Result: Tokens=0, Cost=0 in all runs.

## Fix

Added symlink resolution in `internal/wrapper/snapshot.go`:

```go
func getClaudeProjectDir(cwd string) string {
    // Resolve symlinks (e.g., /tmp -> /private/tmp on macOS)
    realCwd, err := filepath.EvalSymlinks(cwd)
    if err != nil {
        realCwd = cwd
    }
    // ... rest of function
}
```

## Additional Fix

Added `claude-opus-4-7` to model pricing map in `internal/wrapper/tailer.go`.

## Testing

After fix:
```
Tokens: 24 in / 2517 out
Cost: $1.31
Model: claude-opus-4-7
```

Jira completion comment now shows full details:
- Run statistics (agent, duration, cost, tokens, model)
- Git HEAD before → after
- Files changed
- Commits made

## Files Changed

- `internal/wrapper/snapshot.go` — symlink resolution
- `internal/wrapper/tailer.go` — opus-4-7 pricing

## Lessons Learned

1. Always test with real paths, not symlinked directories
2. macOS `/tmp` → `/private/tmp` symlink is a common gotcha
3. Keep model pricing map updated as new models release
