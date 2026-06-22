-- Passive DNS cache: domain -> resolved IP, learned read-only from DNS answers
-- crossing the bridge. Short TTL; feeds derived bad-IPs into the IP enforcer.
CREATE TABLE IF NOT EXISTS snoop (
    id         INTEGER PRIMARY KEY,
    domain     TEXT    NOT NULL,
    ip         TEXT    NOT NULL,
    seen_at    INTEGER NOT NULL DEFAULT 0,
    expires_at INTEGER NOT NULL DEFAULT 0,
    UNIQUE(domain, ip)
);
CREATE INDEX IF NOT EXISTS idx_snoop_expires ON snoop(expires_at);
