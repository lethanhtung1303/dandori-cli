# dandori-cli FAQ & Troubleshooting

## Common Issues

### Token/Cost Shows $0.00

**Symptom:** Run completes but `dandori analytics runs` shows `$0.00` cost.

**Cause:** Session log not found or wrong directory.

**Fix:** Run `dandori run` from the same directory where Claude Code stores sessions:
```bash
# Check Claude project directory exists
ls ~/.claude/projects/-$(pwd | tr '/' '-')/

# Run from project root, not /tmp or other locations
cd /path/to/your/project
./bin/dandori run --task PROJ-1 -- claude "..."
```

### Jira Connection Failed

**Symptom:** `jira API error: 401 - Unauthorized`

**Fix:**
1. Verify API token is correct (not password)
2. Check `cloud: true` for Atlassian Cloud
3. Ensure user email matches Jira account

```bash
# Test connection
curl -u "email@example.com:API_TOKEN" \
  https://YOUR-DOMAIN.atlassian.net/rest/api/2/myself
```

### Confluence Write Failed

**Symptom:** `confluence API error: 404`

**Fix:**
1. Verify `space_key` exists
2. Check `reports_parent_page_id` if set
3. Ensure user has write permission

```yaml
confluence:
  space_key: "PROJ"  # Not "Project Name"
  # reports_parent_page_id: "12345"  # Optional
```

### Task Transition Failed

**Symptom:** `Warning: could not transition`

**Cause:** Issue already in target status or workflow doesn't allow transition.

**Fix:** Check issue status in Jira. Transitions are workflow-dependent.

### Database Locked

**Symptom:** `database is locked`

**Fix:** Only one dandori process should write at a time. Kill other processes:
```bash
pkill -f "dandori run"
```

## Configuration Questions

### How to use multiple agents?

Change agent name per workstation:
```yaml
agent:
  name: "alpha"  # or "beta", "gamma"
```

### How to track without Jira?

Omit `--task` flag:
```bash
./bin/dandori run -- claude "do something"
```
Run is tracked locally but not linked to Jira.

### How to disable Confluence auto-post?

```yaml
confluence:
  auto_post: false
```

### Where is data stored?

| Data | Location |
|------|----------|
| Config | `~/.dandori/config.yaml` |
| Runs/Events | `~/.dandori/local.db` |
| Page cache | `~/.dandori/cache/` |

## Command Reference

| Command | Purpose |
|---------|---------|
| `dandori init` | Create config + database |
| `dandori run` | Execute agent with tracking |
| `dandori task start/done/info` | Manage Jira task lifecycle |
| `dandori jira-sync` | Sync run status to Jira |
| `dandori conf-write` | Write report to Confluence |
| `dandori analytics` | View local analytics |
| `dandori dashboard` | Open web dashboard |
| `dandori status` | Show recent runs |
| `dandori sync` | Upload to server (if configured) |

## Debug Mode

Enable verbose logging:
```bash
./bin/dandori -v run --task PROJ-1 -- claude "..."
```

Or set environment:
```bash
export DANDORI_VERBOSE=true
```

## Reset Local Data

```bash
rm ~/.dandori/local.db
./bin/dandori init
```

## Still Stuck?

1. Check `docs/devlog.md` for known issues
2. Run tests: `go test ./...`
3. Open issue: https://github.com/phuc-nt/dandori-cli/issues
