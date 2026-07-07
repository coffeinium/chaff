-- Групповые политики (ОПАСНЫЙ ЭКСПЕРИМЕНТ). Модуль по умолчанию выключен.
CREATE TABLE IF NOT EXISTS policy_groups (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT    NOT NULL UNIQUE,
    action     TEXT    NOT NULL DEFAULT 'block',
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

INSERT OR IGNORE INTO modules (name, enabled) VALUES ('grouppolicy', 0);
