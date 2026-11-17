CREATE TABLE requests (
    id                INTEGER PRIMARY KEY,
    source            TEXT    NOT NULL,
    host              TEXT    NOT NULL,
    port              INTEGER NOT NULL,
    tls               INTEGER NOT NULL,
    method            TEXT    NOT NULL,
    path              TEXT    NOT NULL,
    query             TEXT    NOT NULL DEFAULT '',
    ext               TEXT    NOT NULL DEFAULT '',
    raw               BLOB    NOT NULL,
    body_offset       INTEGER NOT NULL DEFAULT 0,
    len               INTEGER NOT NULL,
    created_at        INTEGER NOT NULL,
    edited_from       INTEGER REFERENCES requests(id) ON DELETE SET NULL,
    color             TEXT,
    note              TEXT,
    is_dropped        INTEGER NOT NULL DEFAULT 0,
    truncated         INTEGER NOT NULL DEFAULT 0,
    plugin_id         TEXT,
    automate_attack_id INTEGER
) STRICT;
CREATE INDEX requests_host_idx    ON requests(host);
CREATE INDEX requests_created_idx ON requests(created_at);
CREATE INDEX requests_source_idx  ON requests(source);
CREATE INDEX requests_method_idx  ON requests(method);

CREATE TABLE responses (
    request_id   INTEGER PRIMARY KEY REFERENCES requests(id) ON DELETE CASCADE,
    code         INTEGER NOT NULL,
    raw          BLOB    NOT NULL,
    body_offset  INTEGER NOT NULL DEFAULT 0,
    len          INTEGER NOT NULL,
    roundtrip_ms INTEGER NOT NULL DEFAULT 0,
    created_at   INTEGER NOT NULL,
    edited_from  INTEGER
) STRICT;
CREATE INDEX responses_code_idx ON responses(code);

CREATE TABLE request_headers (
    request_id INTEGER NOT NULL REFERENCES requests(id) ON DELETE CASCADE,
    idx        INTEGER NOT NULL,
    name       TEXT    NOT NULL,
    name_lc    TEXT    NOT NULL,
    value      TEXT    NOT NULL,
    PRIMARY KEY (request_id, idx)
) STRICT, WITHOUT ROWID;
CREATE INDEX request_headers_name_idx ON request_headers(name_lc);

CREATE TABLE response_headers (
    request_id INTEGER NOT NULL REFERENCES requests(id) ON DELETE CASCADE,
    idx        INTEGER NOT NULL,
    name       TEXT    NOT NULL,
    name_lc    TEXT    NOT NULL,
    value      TEXT    NOT NULL,
    PRIMARY KEY (request_id, idx)
) STRICT, WITHOUT ROWID;
CREATE INDEX response_headers_name_idx ON response_headers(name_lc);

CREATE TABLE ws_streams (
    id         INTEGER PRIMARY KEY,
    request_id INTEGER REFERENCES requests(id) ON DELETE SET NULL,
    host       TEXT    NOT NULL,
    port       INTEGER NOT NULL,
    path       TEXT    NOT NULL,
    tls        INTEGER NOT NULL,
    created_at INTEGER NOT NULL,
    closed_at  INTEGER
) STRICT;

CREATE TABLE ws_messages (
    id          INTEGER PRIMARY KEY,
    stream_id   INTEGER NOT NULL REFERENCES ws_streams(id) ON DELETE CASCADE,
    direction   TEXT    NOT NULL,
    format      TEXT    NOT NULL,
    raw         BLOB    NOT NULL,
    len         INTEGER NOT NULL,
    created_at  INTEGER NOT NULL,
    edited_from INTEGER
) STRICT;
CREATE INDEX ws_messages_stream_idx ON ws_messages(stream_id);

CREATE TABLE scope_presets (
    id        INTEGER PRIMARY KEY,
    name      TEXT NOT NULL,
    allow_json TEXT NOT NULL DEFAULT '[]',
    deny_json  TEXT NOT NULL DEFAULT '[]'
) STRICT;

CREATE TABLE filter_presets (
    id         INTEGER PRIMARY KEY,
    name       TEXT NOT NULL,
    alias      TEXT,
    clause     TEXT NOT NULL,
    sort_order INTEGER NOT NULL DEFAULT 0
) STRICT;

CREATE TABLE ws_filter_presets (
    id         INTEGER PRIMARY KEY,
    name       TEXT NOT NULL,
    alias      TEXT,
    clause     TEXT NOT NULL,
    sort_order INTEGER NOT NULL DEFAULT 0
) STRICT;

CREATE TABLE mr_collections (
    id         INTEGER PRIMARY KEY,
    name       TEXT    NOT NULL,
    sort_order INTEGER NOT NULL DEFAULT 0
) STRICT;

CREATE TABLE mr_rules (
    id            INTEGER PRIMARY KEY,
    collection_id INTEGER NOT NULL REFERENCES mr_collections(id) ON DELETE CASCADE,
    name          TEXT    NOT NULL,
    enabled       INTEGER NOT NULL DEFAULT 1,
    definition_json TEXT  NOT NULL DEFAULT '{}',
    sort_order    INTEGER NOT NULL DEFAULT 0
) STRICT;

CREATE TABLE replay_collections (
    id         INTEGER PRIMARY KEY,
    name       TEXT    NOT NULL,
    sort_order INTEGER NOT NULL DEFAULT 0
) STRICT;

CREATE TABLE replay_sessions (
    id            INTEGER PRIMARY KEY,
    collection_id INTEGER REFERENCES replay_collections(id) ON DELETE SET NULL,
    kind          TEXT    NOT NULL CHECK (kind IN ('http', 'ws', 'pipeline')),
    name          TEXT    NOT NULL,
    state_json    TEXT    NOT NULL DEFAULT '{}',
    sort_order    INTEGER NOT NULL DEFAULT 0,
    created_at    INTEGER NOT NULL
) STRICT;

CREATE TABLE replay_entries (
    id         INTEGER PRIMARY KEY,
    session_id INTEGER NOT NULL REFERENCES replay_sessions(id) ON DELETE CASCADE,
    request_id INTEGER NOT NULL REFERENCES requests(id) ON DELETE CASCADE,
    created_at INTEGER NOT NULL
) STRICT;
CREATE INDEX replay_entries_session_idx ON replay_entries(session_id);

CREATE TABLE automate_sessions (
    id          INTEGER PRIMARY KEY,
    name        TEXT    NOT NULL,
    config_json TEXT    NOT NULL DEFAULT '{}',
    created_at  INTEGER NOT NULL
) STRICT;

CREATE TABLE automate_attacks (
    id          INTEGER PRIMARY KEY,
    session_id  INTEGER NOT NULL REFERENCES automate_sessions(id) ON DELETE CASCADE,
    status      TEXT    NOT NULL,
    config_json TEXT    NOT NULL DEFAULT '{}',
    created_at  INTEGER NOT NULL,
    finished_at INTEGER
) STRICT;

CREATE TABLE automate_results (
    id              INTEGER PRIMARY KEY,
    attack_id       INTEGER NOT NULL REFERENCES automate_attacks(id) ON DELETE CASCADE,
    request_id      INTEGER REFERENCES requests(id) ON DELETE SET NULL,
    payloads_json   TEXT NOT NULL DEFAULT '[]',
    extractors_json TEXT NOT NULL DEFAULT '[]',
    error           TEXT
) STRICT;
CREATE INDEX automate_results_attack_idx ON automate_results(attack_id);

CREATE TABLE findings (
    id             INTEGER PRIMARY KEY,
    request_id     INTEGER REFERENCES requests(id) ON DELETE SET NULL,
    reporter       TEXT    NOT NULL,
    title          TEXT    NOT NULL,
    description_md TEXT    NOT NULL DEFAULT '',
    dedupe_key     TEXT,
    created_at     INTEGER NOT NULL,
    UNIQUE (reporter, dedupe_key)
) STRICT;

CREATE TABLE exports (
    id         INTEGER PRIMARY KEY,
    name       TEXT    NOT NULL,
    format     TEXT    NOT NULL,
    rows       INTEGER NOT NULL DEFAULT 0,
    size       INTEGER NOT NULL DEFAULT 0,
    path       TEXT    NOT NULL DEFAULT '',
    status     TEXT    NOT NULL,
    created_at INTEGER NOT NULL
) STRICT;

CREATE TABLE environments (
    id       INTEGER PRIMARY KEY,
    name     TEXT    NOT NULL,
    selected INTEGER NOT NULL DEFAULT 0
) STRICT;

CREATE TABLE env_vars (
    env_id INTEGER NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    name   TEXT    NOT NULL,
    value  TEXT    NOT NULL,
    secret INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (env_id, name)
) STRICT;

CREATE TABLE assistant_sessions (
    id         INTEGER PRIMARY KEY,
    title      TEXT    NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL
) STRICT;

CREATE TABLE assistant_messages (
    id         INTEGER PRIMARY KEY,
    session_id INTEGER NOT NULL REFERENCES assistant_sessions(id) ON DELETE CASCADE,
    role       TEXT    NOT NULL,
    content    TEXT    NOT NULL,
    created_at INTEGER NOT NULL
) STRICT;
CREATE INDEX assistant_messages_session_idx ON assistant_messages(session_id);

CREATE TABLE intercept_settings (
    key        TEXT PRIMARY KEY,
    value_json TEXT NOT NULL
) STRICT;

CREATE TABLE ui_state (
    key        TEXT PRIMARY KEY,
    value_json TEXT NOT NULL
) STRICT;
