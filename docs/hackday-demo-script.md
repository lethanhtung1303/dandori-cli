# Hackday Demo Script — Dandori CLI

Blog slide → 1-line command → narration. Target: **≤ 3 minutes** total.

## Pre-flight (off-stage, before joining call)

```bash
make build
./scripts/hackday-rehearsal.sh dry    # must finish < 30s, no errors
```

Confirm:
- `bin/dandori` exists and responds to `--version`
- `~/.dandori/active_db` does NOT exist (rehearsal `--restore`d it)
- Jira/Confluence env vars loaded (if live)

### Sandbox for live runs

Live-mode `task run` invokes Claude with file-write permission. The
rehearsal script creates `demo-workspace/<YYMMDD-HHmm>-<mode>/` and
`cd`s Claude into it, so artifacts stay out of the source tree. Each
session is kept for post-mortem review; prune manually when the folder
grows large. See [`../demo-workspace/README.md`](../demo-workspace/README.md).

---

## Stage 1 — "One command, real numbers" (0:00 → 0:30)

**Slide:** *"Who spent what, last sprint?"*

**Command:**
```bash
dandori demo --reset --seed --use
```

**Say:**
> Fresh demo DB. 28 seeded runs spanning three engineers: Alice pairing
> with agent alpha, Bob working solo (human-only), Carol pairing with beta.
> Matches the blog scenario exactly.

**Expect output:**
```
Demo DB reset (runs/events/quality_metrics cleared).
Seeded blog scenario: Alice+alpha (12), Bob human-only (9), Carol+beta (7).
Active DB now: /Users/you/.dandori/demo.db
```

---

## Stage 2 — "4 blocks, 1 command" (0:30 → 1:15)

**Slide:** *"Cost · Leaderboard · Quality · Alerts — one screen"*

**Command:**
```bash
dandori analytics all --since 30
```

**Say:**
> The same view a Head-of-Engineering gets at Monday stand-up — cost per
> engineer, the mixed human + agent leaderboard, per-agent quality deltas,
> and threshold alerts. No dashboard, no BI tool — just the CLI reading
> the same local SQLite every run wrote to.

**Highlight:**
- Block 1: Alice $13.50 > Carol $4.69 > Bob $0 (humans don't cost tokens)
- Block 2: Bob's agent column reads `(human)` — first-class support
- Block 3: alpha Δtests +15 beats beta +5
- Block 4: `(none)` — seeded data is within thresholds

---

## Stage 3 — "Regroup on the fly" (1:15 → 1:45)

**Slide:** *"Cut the same data by engineer, by department"*

**Commands:**
```bash
dandori analytics cost --by engineer
dandori analytics cost --by department
```

**Say:**
> Same runs, different lens. Department shows Platform (Alice + Bob) at
> $13.50 vs Growth (Carol) at $4.69. Manager view, no new data needed.

---

## Stage 4 — "A real Claude run" (1:45 → 2:45) — **LIVE only**

**Slide:** *"Wrap Claude once, capture everything"*

**Command:**
```bash
dandori task run CLITEST-1
```

**Say:**
> This is the outer-harness moment. Claude runs inside `dandori task run`
> against a real Jira ticket. When the session ends, the wrapper drains
> the session JSONL, captures tokens + cost, writes the run row.
>
> Pre-fix this number was zero on fast tasks — the session log wasn't
> flushed before we tore down the tailer. Phase 01 fixed the race: we
> wait up to 10 seconds post-exit, idle-grace 750ms.

**Verify:**
```bash
dandori analytics runs --limit 1
```

Token count **must be > 0** — this is the Phase-01 proof.

---

## Stage 5 — "Close the loop" (2:45 → 3:00)

**Command:**
```bash
dandori demo --restore
```

**Say:**
> Pointer flipped back. Real DB untouched. Demo was idempotent.

---

## Fallback / troubleshooting

| Symptom | Action |
|---|---|
| `no cost data yet` after seed | `DANDORI_DB` env overrides `active_db`; unset it, re-run |
| `tokens = 0` on live run | retry once; tailer timeout = 10s so slow flushes need ≤ 10s — bump via `~/.dandori/config.yaml` `wrapper.post_exit_timeout` |
| `analytics all` missing QUALITY GATES | seeded data should always produce 3 agent rows — if empty, migration v3 didn't run (`dandori demo --reset --seed` forces migrate) |
| Rehearsal > 30s dry | profile: likely Jira fetch on `task run --dry-run`. Offline mode: remove stage 2 dry Jira step |

## Timing breakdown (live run, target ≤ 180s)

| Stage | Budget |
|---|---|
| 1 seed + use | 3s |
| 2 analytics all | 2s |
| 3 group-by engineer + dept | 3s |
| 4 task run CLITEST-1 (live Claude) | 150s |
| 5 analytics runs + restore | 2s |
| **Buffer** | 20s |

## Rehearsal cadence

- Dry rehearsal: run before every practice session (`./scripts/hackday-rehearsal.sh dry`)
- Live rehearsal: run 3× back-to-back the day before — measure variance of Stage 4 (Claude latency is the only wildcard)
- If p95 Stage 4 > 150s, swap CLITEST-1 for a smaller task
