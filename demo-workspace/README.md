# demo-workspace

Sandbox for Claude runs during demo/test rehearsals. Not production code.

## Layout

Each rehearsal creates one subdir:

```
demo-workspace/
├── 260425-0910-live/     # YYMMDD-HHmm-<mode>
│   ├── ...any files Claude wrote during the run
│   └── ...
├── 260425-0915-dry/
└── ...
```

Session dirs are kept for post-mortem review (diff, prompt iteration,
token-cost comparison). Nothing auto-cleans — prune manually when the
folder grows too large.

## Why a dedicated folder

`dandori task run CLITEST-1` invokes Claude headless with file-write
permissions. Without a sandbox, Claude would modify the `dandori-cli`
source tree itself. Running inside a session dir isolates artifacts and
keeps `git status` in the repo root clean.

## Conventions

- Mode tag: `live` = real Claude, `dry` = context-fetch only.
- Git-ignored by `.gitignore` (only `README.md` + `.gitignore` tracked).
- Safe to `rm -rf demo-workspace/<session-dir>` at any time.
