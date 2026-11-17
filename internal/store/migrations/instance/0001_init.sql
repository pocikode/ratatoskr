CREATE TABLE settings (
    key        TEXT PRIMARY KEY,
    value_json TEXT NOT NULL
) STRICT;

CREATE TABLE projects (
    id             INTEGER PRIMARY KEY,
    name           TEXT    NOT NULL,
    path           TEXT    NOT NULL,
    created_at     INTEGER NOT NULL,
    last_opened_at INTEGER,
    is_default     INTEGER NOT NULL DEFAULT 0
) STRICT;

CREATE TABLE ca (
    id          INTEGER PRIMARY KEY CHECK (id = 1),
    cert_pem    TEXT NOT NULL,
    key_pem_enc BLOB NOT NULL
) STRICT;

CREATE TABLE workflows (
    id          INTEGER PRIMARY KEY,
    kind        TEXT    NOT NULL,
    name        TEXT    NOT NULL,
    description TEXT    NOT NULL DEFAULT '',
    enabled     INTEGER NOT NULL DEFAULT 1,
    graph_json  TEXT    NOT NULL DEFAULT '{}',
    shortcut    TEXT,
    updated_at  INTEGER NOT NULL
) STRICT;

CREATE TABLE plugins (
    id            TEXT PRIMARY KEY,
    manifest_json TEXT    NOT NULL,
    enabled       INTEGER NOT NULL DEFAULT 1,
    installed_at  INTEGER NOT NULL
) STRICT;

CREATE TABLE plugin_storage (
    plugin_id TEXT NOT NULL REFERENCES plugins(id) ON DELETE CASCADE,
    key       TEXT NOT NULL,
    value     BLOB NOT NULL,
    PRIMARY KEY (plugin_id, key)
) STRICT;

CREATE TABLE hosted_files (
    id         INTEGER PRIMARY KEY,
    name       TEXT    NOT NULL,
    size       INTEGER NOT NULL,
    sha256     TEXT    NOT NULL,
    path       TEXT    NOT NULL,
    created_at INTEGER NOT NULL
) STRICT;

CREATE TABLE env_global (
    name   TEXT PRIMARY KEY,
    value  TEXT    NOT NULL,
    secret INTEGER NOT NULL DEFAULT 0
) STRICT;

CREATE TABLE ai_providers (
    id          INTEGER PRIMARY KEY,
    kind        TEXT NOT NULL,
    base_url    TEXT NOT NULL,
    api_key_enc BLOB,
    models_json TEXT NOT NULL DEFAULT '[]'
) STRICT;

CREATE TABLE shortcuts (
    command_id TEXT PRIMARY KEY,
    binding    TEXT NOT NULL
) STRICT;
