CREATE TABLE IF NOT EXISTS hostnames (
    kind     TEXT    NOT NULL,
    key      TEXT    NOT NULL,
    hostname TEXT    NOT NULL,
    via      TEXT    NOT NULL DEFAULT '',
    seen_at  INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (kind, key)
);
