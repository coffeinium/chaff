CREATE TABLE IF NOT EXISTS web_tokens (
    id         INTEGER PRIMARY KEY,
    name       TEXT    NOT NULL DEFAULT '',
    hash       TEXT    NOT NULL UNIQUE,
    created_at INTEGER NOT NULL DEFAULT 0,
    expires_at INTEGER NOT NULL DEFAULT 0,
    last_used  INTEGER NOT NULL DEFAULT 0
);
