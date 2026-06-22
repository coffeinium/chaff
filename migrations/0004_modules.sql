-- Module on/off overrides. Absence of a row means "enabled" (default-on), so
-- this table only records explicit operator decisions.
CREATE TABLE IF NOT EXISTS modules (
    name    TEXT PRIMARY KEY,
    enabled INTEGER NOT NULL DEFAULT 1,
    config  TEXT    NOT NULL DEFAULT '{}'
);
