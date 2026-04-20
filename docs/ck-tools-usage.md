# CK Tools Usage in dandori-cli

> Summary of Claude Kit (CK) tools and patterns used during development.

## Tools Used

### 1. Subagents (Agent Tool)

| Agent | Usage | When |
|-------|-------|------|
| `Explore` | Codebase exploration | Finding files, understanding structure |
| `planner` | Implementation planning | Before starting new features |
| `tester` | Running tests | After implementation |
| `code-reviewer` | Code review | Before commits |
| `debugger` | Investigating issues | When tests fail |
| `researcher` | Technical research | Exploring best practices |
| `docs-manager` | Documentation updates | After feature completion |

### 2. Background Tasks (Task Tool)

```bash
# Used for long-running operations
Task(run_in_background=true):
  - E2E test suites
  - Build verification
  - Watch daemon testing
```

### 3. Skills Activated

| Skill | Purpose |
|-------|---------|
| `ck:autoresearch` | Autonomous optimization loops (referenced, not heavily used) |
| `ck:scenario` | Edge case exploration (referenced in planning) |

### 4. Rules & Protocols

From `.claude/rules/`:

| Rule File | Applied |
|-----------|---------|
| `development-rules.md` | YAGNI, KISS, DRY principles |
| `primary-workflow.md` | Plan → Implement → Test → Review cycle |
| `orchestration-protocol.md` | Subagent delegation patterns |
| `documentation-management.md` | Docs update after features |
| `team-coordination-rules.md` | (Not used - solo project) |

### 5. Patterns Applied

#### Plan-First Development
```
1. Create plan in plans/{date}-{slug}/
2. Break into phases (phase-01, phase-02, ...)
3. Implement phase by phase
4. Update status in plan.md
```

#### TDD with Real Data
```
1. Clear old test data
2. Run real agent (Claude Code)
3. Verify token/cost capture
4. Check Jira sync
5. Iterate on failures
```

#### Subagent Delegation
```
Main agent (coordinator)
  ├── Explore agent (codebase questions)
  ├── Tester agent (run tests)
  └── Debugger agent (investigate failures)
```

## Tools NOT Used (but available)

| Tool | Why Not Used |
|------|--------------|
| `ck:team` | Solo project, no multi-agent orchestration needed |
| `ai-artist` | No image generation required |
| `agent-browser` | No web scraping needed |
| `bootstrap` | Project already initialized |

## Effectiveness Assessment

| Tool/Pattern | Effectiveness | Notes |
|--------------|---------------|-------|
| Plan-first | ⭐⭐⭐⭐⭐ | Clear phases, trackable progress |
| Subagents | ⭐⭐⭐⭐ | Good for parallel exploration |
| Background tasks | ⭐⭐⭐⭐ | Essential for long E2E tests |
| TDD with real data | ⭐⭐⭐⭐⭐ | Caught symlink bug |
| Rules files | ⭐⭐⭐⭐ | Consistent code style |

## Lessons Learned

### What Worked Well

1. **Plan structure** — `plans/{date}-{slug}/plan.md` với phases riêng biệt
2. **Devlog** — Ghi lại decisions và issues trong `docs/devlog/`
3. **Real integration testing** — Test với Jira/Confluence thật, không mock
4. **Subagent for exploration** — Explore agent tìm files nhanh hơn manual grep

### What Could Improve

1. **Skill library** — Chưa tận dụng reusable prompts
2. **Autoresearch** — Có thể dùng cho coverage optimization
3. **Team coordination** — Nếu có multi-agent, cần rules rõ hơn

## Recommended Reading

For future developers using CK tools:

1. `.claude/rules/development-rules.md` — Core principles
2. `.claude/rules/primary-workflow.md` — Development flow
3. `.claude/rules/orchestration-protocol.md` — How to use subagents
4. `plans/260418-1301-dandori-cli/plan.md` — Example of good planning

## Quick Commands

```bash
# Explore codebase (via Agent tool)
Agent(subagent_type="Explore", prompt="Find all files related to Jira integration")

# Run tests (via Agent tool)  
Agent(subagent_type="tester", prompt="Run go test ./... and report failures")

# Background task
Agent(run_in_background=true, prompt="Run E2E tests")
```
