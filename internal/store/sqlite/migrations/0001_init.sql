-- 初始 schema：文档模式存储（data 列保存完整 JSON），索引列便于查询
CREATE TABLE IF NOT EXISTS servers (
    id              TEXT PRIMARY KEY,
    protocol        TEXT NOT NULL DEFAULT '',
    name            TEXT NOT NULL DEFAULT '',
    server          TEXT NOT NULL DEFAULT '',
    server_port     INTEGER NOT NULL DEFAULT 0,
    last_latency_ms INTEGER,
    added_at        TEXT NOT NULL,
    data            TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_servers_protocol ON servers(protocol);

CREATE TABLE IF NOT EXISTS groups (
    id   TEXT PRIMARY KEY,
    name TEXT NOT NULL DEFAULT '',
    type TEXT NOT NULL DEFAULT 'selector',
    data TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS subscriptions (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL DEFAULT '',
    url           TEXT NOT NULL DEFAULT '',
    last_refresh  TEXT,
    last_status   TEXT,
    data          TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS routing_rules (
    id         TEXT PRIMARY KEY,
    profile_id TEXT NOT NULL DEFAULT '',
    ord        INTEGER NOT NULL DEFAULT 0,
    type       TEXT NOT NULL DEFAULT '',
    outbound   TEXT NOT NULL DEFAULT '',
    data       TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_rules_profile ON routing_rules(profile_id);

CREATE TABLE IF NOT EXISTS rule_sets (
    id   TEXT PRIMARY KEY,
    tag  TEXT NOT NULL DEFAULT '',
    type TEXT NOT NULL DEFAULT 'local',
    data TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS profiles (
    id   TEXT PRIMARY KEY,
    name TEXT NOT NULL DEFAULT '',
    mode TEXT NOT NULL DEFAULT 'normal',
    data TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

-- 迁移版本追踪
CREATE TABLE IF NOT EXISTS schema_version (
    version INTEGER PRIMARY KEY,
    applied TEXT NOT NULL
);
