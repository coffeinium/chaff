-- Enforcement hits for observability/tuning (`chaff log`).
CREATE TABLE IF NOT EXISTS hits (
    id        INTEGER PRIMARY KEY,
    ts        INTEGER NOT NULL,
    layer     TEXT    NOT NULL,           -- ip|sni|http|dns
    indicator TEXT    NOT NULL,
    src_ip    TEXT    NOT NULL DEFAULT '',
    detail    TEXT    NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_hits_ts ON hits(ts);
