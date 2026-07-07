-- Групповые политики (ОПАСНЫЙ ЭКСПЕРИМЕНТ). Модуль по умолчанию выключен.
-- Группа — именованный набор машин (по MAC или имени хоста) со своими
-- правилами (ip/cidr/домен/url), действующими только на участников.
CREATE TABLE IF NOT EXISTS policy_groups (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT    NOT NULL UNIQUE,
    enabled    INTEGER NOT NULL DEFAULT 0,
    note       TEXT    NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL DEFAULT 0
);

-- (kind, value) уникальны глобально: одна машина не может быть больше чем в одной группе.
CREATE TABLE IF NOT EXISTS policy_members (
    group_id INTEGER NOT NULL REFERENCES policy_groups(id) ON DELETE CASCADE,
    kind     TEXT    NOT NULL,
    value    TEXT    NOT NULL,
    PRIMARY KEY (kind, value)
);

CREATE INDEX IF NOT EXISTS idx_policy_members_group ON policy_members(group_id);

CREATE TABLE IF NOT EXISTS policy_rules (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    group_id   INTEGER NOT NULL REFERENCES policy_groups(id) ON DELETE CASCADE,
    value      TEXT    NOT NULL,
    kind       TEXT    NOT NULL,
    action     TEXT    NOT NULL DEFAULT 'block',
    scope      TEXT    NOT NULL DEFAULT 'domain',
    note       TEXT    NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL DEFAULT 0,
    UNIQUE (group_id, value, kind)
);

CREATE INDEX IF NOT EXISTS idx_policy_rules_group ON policy_rules(group_id);

INSERT OR IGNORE INTO modules (name, enabled) VALUES ('grouppolicy', 0);
