CREATE TABLE IF NOT EXISTS modules (
    name    TEXT PRIMARY KEY,
    enabled INTEGER NOT NULL DEFAULT 1,
    config  TEXT    NOT NULL DEFAULT '{}'
);
