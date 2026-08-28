CREATE TABLE context_pages (
    page_id TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL,
    page_type TEXT NOT NULL,
    scope TEXT NOT NULL,
    source_ref TEXT NOT NULL DEFAULT '',
    object_ref TEXT NOT NULL,
    token_estimate INTEGER NOT NULL CHECK (token_estimate >= 0),
    importance REAL NOT NULL CHECK (importance >= 0 AND importance <= 1),
    confidence REAL NOT NULL CHECK (confidence >= 0 AND confidence <= 1),
    created_at TEXT NOT NULL,
    last_accessed_at TEXT NOT NULL,
    pinned_until TEXT,
    superseded_by TEXT,
    compacted_by TEXT,
    summary_of_json TEXT NOT NULL DEFAULT '[]',
    search_text TEXT NOT NULL DEFAULT ''
);

CREATE INDEX idx_context_pages_agent_scope_access
    ON context_pages(agent_id, scope, last_accessed_at DESC);
CREATE INDEX idx_context_pages_agent_type
    ON context_pages(agent_id, page_type);
CREATE INDEX idx_context_pages_cold
    ON context_pages(agent_id, superseded_by, compacted_by);

CREATE VIRTUAL TABLE context_pages_fts USING fts5(
    page_id UNINDEXED,
    search_text,
    tokenize = 'unicode61'
);

CREATE TRIGGER context_pages_fts_insert
AFTER INSERT ON context_pages
BEGIN
    INSERT INTO context_pages_fts(rowid, page_id, search_text)
    VALUES (new.rowid, new.page_id, new.search_text);
END;

CREATE TRIGGER context_pages_fts_delete
AFTER DELETE ON context_pages
BEGIN
    DELETE FROM context_pages_fts WHERE rowid = old.rowid;
END;

CREATE TRIGGER context_pages_fts_update
AFTER UPDATE OF search_text ON context_pages
BEGIN
    DELETE FROM context_pages_fts WHERE rowid = old.rowid;
    INSERT INTO context_pages_fts(rowid, page_id, search_text)
    VALUES (new.rowid, new.page_id, new.search_text);
END;

CREATE TABLE context_leases (
    lease_id TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL,
    page_id TEXT NOT NULL,
    reason TEXT NOT NULL,
    remaining_builds INTEGER NOT NULL CHECK (remaining_builds > 0),
    expires_at TEXT,
    hard_pin INTEGER NOT NULL CHECK (hard_pin IN (0, 1)),
    created_at TEXT NOT NULL,
    UNIQUE(agent_id, page_id),
    FOREIGN KEY(page_id) REFERENCES context_pages(page_id) ON DELETE CASCADE
);

CREATE INDEX idx_context_leases_agent_active
    ON context_leases(agent_id, remaining_builds, expires_at);

CREATE TABLE context_manifests (
    invocation_id TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL,
    object_ref TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX idx_context_manifests_agent_created
    ON context_manifests(agent_id, created_at DESC);
