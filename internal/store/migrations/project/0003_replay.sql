ALTER TABLE replay_collections ADD COLUMN is_default INTEGER NOT NULL DEFAULT 0 CHECK (is_default IN (0, 1));
CREATE UNIQUE INDEX replay_collections_default_idx ON replay_collections(is_default) WHERE is_default = 1;

-- Connection, options and colour use state_json; drafts stay byte-exact BLOBs.
ALTER TABLE replay_sessions ADD COLUMN draft BLOB NOT NULL DEFAULT X'';

-- Never reuse an entry ID: cursors must survive deletion of their boundary entry.
CREATE TABLE replay_entries_new (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id INTEGER NOT NULL REFERENCES replay_sessions(id) ON DELETE CASCADE,
    request_id INTEGER NOT NULL REFERENCES requests(id) ON DELETE CASCADE,
    created_at INTEGER NOT NULL,
    error      TEXT
) STRICT;
INSERT INTO replay_entries_new(id, session_id, request_id, created_at)
    SELECT id, session_id, request_id, created_at FROM replay_entries;
DROP TABLE replay_entries;
ALTER TABLE replay_entries_new RENAME TO replay_entries;
CREATE INDEX replay_entries_session_idx ON replay_entries(session_id, id DESC);
CREATE INDEX replay_entries_request_idx ON replay_entries(request_id);
