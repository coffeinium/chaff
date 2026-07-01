CREATE TABLE IF NOT EXISTS indicators (
    id         INTEGER PRIMARY KEY,
    value      TEXT    NOT NULL,
    kind       TEXT    NOT NULL,
    action     TEXT    NOT NULL DEFAULT 'block',
    scope      TEXT    NOT NULL DEFAULT 'domain',
    threat     TEXT    NOT NULL DEFAULT '',
    note       TEXT    NOT NULL DEFAULT '',
    source_id  INTEGER NOT NULL DEFAULT 0,
    first_seen INTEGER NOT NULL DEFAULT 0,
    last_seen  INTEGER NOT NULL DEFAULT 0,
    expires_at INTEGER NOT NULL DEFAULT 0,
    enabled    INTEGER NOT NULL DEFAULT 1,
    UNIQUE(value, kind, source_id)
);
CREATE INDEX IF NOT EXISTS idx_indicators_kind ON indicators(kind, enabled);
