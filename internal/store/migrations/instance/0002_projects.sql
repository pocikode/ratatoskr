-- Project IDs are GraphQL node IDs and directory names: they must never be
-- reused, hence AUTOINCREMENT. is_temporary marks a project removed on clean
-- shutdown unless it was renamed (saved).
CREATE TABLE projects_new (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    name           TEXT    NOT NULL,
    path           TEXT    NOT NULL,
    created_at     INTEGER NOT NULL,
    last_opened_at INTEGER,
    is_default     INTEGER NOT NULL DEFAULT 0,
    is_temporary   INTEGER NOT NULL DEFAULT 0
) STRICT;

INSERT INTO projects_new (id, name, path, created_at, last_opened_at, is_default)
    SELECT id, name, path, created_at, last_opened_at, is_default FROM projects;

DROP TABLE projects;
ALTER TABLE projects_new RENAME TO projects;
