package db

const SchemaVersion = 2

const SchemaSQL = `
CREATE TABLE IF NOT EXISTS schema_version (
    version INTEGER PRIMARY KEY,
    applied_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS runs (
    id TEXT PRIMARY KEY,
    jira_issue_key TEXT,
    jira_sprint_id TEXT,
    agent_name TEXT NOT NULL,
    agent_type TEXT NOT NULL DEFAULT 'claude_code',
    user TEXT NOT NULL,
    workstation_id TEXT NOT NULL,
    cwd TEXT,
    git_remote TEXT,
    git_head_before TEXT,
    git_head_after TEXT,
    command TEXT,
    started_at TEXT NOT NULL,
    ended_at TEXT,
    duration_sec REAL,
    exit_code INTEGER,
    status TEXT NOT NULL DEFAULT 'running',
    session_id TEXT,
    input_tokens INTEGER DEFAULT 0,
    output_tokens INTEGER DEFAULT 0,
    cache_read_tokens INTEGER DEFAULT 0,
    cache_write_tokens INTEGER DEFAULT 0,
    model TEXT,
    cost_usd REAL DEFAULT 0,
    synced INTEGER DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id TEXT NOT NULL REFERENCES runs(id),
    layer INTEGER NOT NULL,
    event_type TEXT NOT NULL,
    data TEXT,
    ts TEXT NOT NULL DEFAULT (datetime('now')),
    synced INTEGER DEFAULT 0
);

CREATE TABLE IF NOT EXISTS audit_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    prev_hash TEXT,
    curr_hash TEXT,
    actor TEXT NOT NULL,
    action TEXT NOT NULL,
    entity_type TEXT,
    entity_id TEXT,
    details TEXT,
    ts TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS quality_metrics (
    run_id TEXT PRIMARY KEY REFERENCES runs(id),
    lint_errors_before INTEGER DEFAULT 0,
    lint_errors_after INTEGER DEFAULT 0,
    lint_warnings_before INTEGER DEFAULT 0,
    lint_warnings_after INTEGER DEFAULT 0,
    tests_total_before INTEGER DEFAULT 0,
    tests_passed_before INTEGER DEFAULT 0,
    tests_failed_before INTEGER DEFAULT 0,
    tests_total_after INTEGER DEFAULT 0,
    tests_passed_after INTEGER DEFAULT 0,
    tests_failed_after INTEGER DEFAULT 0,
    lint_delta INTEGER DEFAULT 0,
    tests_delta INTEGER DEFAULT 0,
    quality_score REAL,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_runs_jira ON runs(jira_issue_key);
CREATE INDEX IF NOT EXISTS idx_runs_synced ON runs(synced) WHERE synced = 0;
CREATE INDEX IF NOT EXISTS idx_runs_status ON runs(status);
CREATE INDEX IF NOT EXISTS idx_events_run_id ON events(run_id);
CREATE INDEX IF NOT EXISTS idx_events_synced ON events(synced) WHERE synced = 0;
CREATE INDEX IF NOT EXISTS idx_audit_log_entity ON audit_log(entity_type, entity_id);
CREATE INDEX IF NOT EXISTS idx_quality_run ON quality_metrics(run_id);
`

// Migration from v1 to v2: add quality_metrics table
const MigrationV1ToV2 = `
CREATE TABLE IF NOT EXISTS quality_metrics (
    run_id TEXT PRIMARY KEY REFERENCES runs(id),
    lint_errors_before INTEGER DEFAULT 0,
    lint_errors_after INTEGER DEFAULT 0,
    lint_warnings_before INTEGER DEFAULT 0,
    lint_warnings_after INTEGER DEFAULT 0,
    tests_total_before INTEGER DEFAULT 0,
    tests_passed_before INTEGER DEFAULT 0,
    tests_failed_before INTEGER DEFAULT 0,
    tests_total_after INTEGER DEFAULT 0,
    tests_passed_after INTEGER DEFAULT 0,
    tests_failed_after INTEGER DEFAULT 0,
    lint_delta INTEGER DEFAULT 0,
    tests_delta INTEGER DEFAULT 0,
    quality_score REAL,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_quality_run ON quality_metrics(run_id);

INSERT OR REPLACE INTO schema_version (version) VALUES (2);
`
