CREATE TABLE IF NOT EXISTS sources (
    id           INTEGER PRIMARY KEY,
    name         TEXT    NOT NULL UNIQUE,
    adapter      TEXT    NOT NULL,
    uri          TEXT    NOT NULL DEFAULT '',
    column_map   TEXT    NOT NULL DEFAULT '{}',
    interval_sec INTEGER NOT NULL DEFAULT 3600,
    enabled      INTEGER NOT NULL DEFAULT 1,
    last_sync    INTEGER NOT NULL DEFAULT 0,
    last_status  TEXT    NOT NULL DEFAULT '',
    last_count   INTEGER NOT NULL DEFAULT 0,
    content_hash TEXT    NOT NULL DEFAULT ''
);
