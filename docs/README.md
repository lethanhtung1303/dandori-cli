# dandori-cli — Handover

> Đọc 3 file này là đủ nắm repo. Tổng ~15 phút.

## 1. Abstract — outer harness là gì (5 phút)

[`../../dandori-pitch/outer-harness.md`](../../dandori-pitch/outer-harness.md)

Công ty 50 dev, hoá đơn AI $10K/tháng, không ai trả lời được tiền đi đâu, agent nào commit code sai, team A/B team nào hiệu quả hơn. Đó không phải lỗi AI — là thiếu **lớp hạ tầng quản lý** quanh AI: cost, audit, task tracking, quality gates, knowledge flow. Lớp đó là **outer harness**. Dandori là hiện thực hóa lớp đó.

## 2. Vision — repo này giải gì (3 phút)

[`../CLAUDE.md`](../CLAUDE.md)

- CLI outer harness: wrap Claude Code, track mọi run, tích hợp Jira + Confluence, analytics đa cấp.
- **Chứng minh**: công ty phần mềm có thể vận hành bởi **PO/PDM + QA + AI agent**, không cần human developer.
- Design principles: wrapper non-negotiable, Jira là task board, Confluence là knowledge store, single binary, offline-capable.

## 3. Current State — đang ở đâu (5 phút)

[`status-assessment.md`](status-assessment.md)

- **v0.3.0** đã publish · 8/8 phase done · **~88% vision**
- 5 Pillars: Cost 100% · Task Tracking 100% · Audit 100% · Quality 75% · Knowledge Flow 55%
- 5 business questions outer-harness đặt ra → 4/5 trả lời được bằng 1 command; câu cuối (knowledge retention) chỉ partial qua Confluence reports.
- **Hackday prep (2026-04-25)**: tailer timing fix · `analytics all` 4-block snapshot · engineer/department group-by · demo seed · rehearsal script (xem [devlog 2026-04-25](devlog/2026-04-25-hackday-prep-and-snapshot-fix.md)).
- Known gaps (ưu tiên giảm dần): multi-agent · context inheritance · skill library · homebrew tap.

---

## Khi cần đào sâu

| Cần gì | Đọc |
|---|---|
| Install + config | [`setup-guide.md`](setup-guide.md) |
| Cách dùng theo use case | [`user-guide.md`](user-guide.md) |
| Gặp lỗi | [`faq.md`](faq.md) |
| Architecture đầy đủ + 8 phase chi tiết | [`../../plans/260418-1301-dandori-cli/plan.md`](../../plans/260418-1301-dandori-cli/plan.md) |
| Lịch sử implement + quyết định | [`devlog/`](devlog/) |
| Release quy trình | [`release-setup.md`](release-setup.md) |
| Hackday demo flow (3 phút, 1 command/stage) | [`hackday-demo-script.md`](hackday-demo-script.md) |
| AI đã build repo này thế nào | [`ck-tools-usage.md`](ck-tools-usage.md) |

## Source code điểm vào

```
cmd/dandori/        → CLI entry + subcommands
internal/runner/    → 3-layer instrumentation, session tailer (trọng tâm)
internal/store/     → SQLite schema + queries
internal/jira/      → Jira client, poller
internal/analytics/ → 8 query types
internal/quality/   → Lint/test delta, commit scoring
```

## Smoke test

```bash
make build && make test && make lint
./bin/dandori version
./bin/dandori task run PROJ-123 --dry-run
```
