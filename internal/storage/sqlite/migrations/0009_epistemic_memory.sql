CREATE TABLE memory_evidence (
    evidence_id TEXT PRIMARY KEY,
    kind TEXT NOT NULL,
    scope_kind TEXT NOT NULL,
    scope_ref TEXT NOT NULL DEFAULT '',
    source_ref TEXT NOT NULL,
    object_ref TEXT NOT NULL DEFAULT '',
    content_hash TEXT NOT NULL,
    observed_at TEXT NOT NULL,
    source_version TEXT NOT NULL DEFAULT '',
    provenance_class TEXT NOT NULL,
    trust_class TEXT NOT NULL,
    sensitivity TEXT NOT NULL
);

CREATE INDEX idx_memory_evidence_source
    ON memory_evidence(source_ref, source_version, observed_at DESC);
CREATE INDEX idx_memory_evidence_scope
    ON memory_evidence(scope_kind, scope_ref, observed_at DESC);
CREATE INDEX idx_memory_evidence_hash
    ON memory_evidence(content_hash);

CREATE TABLE memory_evidence_invalidations (
    evidence_id TEXT PRIMARY KEY,
    reason TEXT NOT NULL,
    replacement_evidence_id TEXT NOT NULL DEFAULT '',
    invalidated_at TEXT NOT NULL,
    FOREIGN KEY(evidence_id) REFERENCES memory_evidence(evidence_id) ON DELETE CASCADE
);

CREATE TABLE memory_source_versions (
    source_ref TEXT NOT NULL,
    version TEXT NOT NULL DEFAULT '',
    content_hash TEXT NOT NULL DEFAULT '',
    observed_at TEXT NOT NULL,
    PRIMARY KEY(source_ref, version, content_hash)
);

CREATE INDEX idx_memory_source_versions_observed
    ON memory_source_versions(source_ref, observed_at DESC);

CREATE TABLE memory_beliefs (
    belief_id TEXT PRIMARY KEY,
    proposition TEXT NOT NULL,
    fingerprint TEXT NOT NULL,
    scope_kind TEXT NOT NULL,
    scope_ref TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL,
    origin_provenance TEXT NOT NULL,
    confidence_json TEXT NOT NULL,
    valid_from TEXT,
    valid_until TEXT,
    as_of_ref TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    last_reviewed_at TEXT,
    version INTEGER NOT NULL CHECK(version > 0),
    speculative INTEGER NOT NULL CHECK(speculative IN (0, 1)),
    origin_fork_id TEXT NOT NULL DEFAULT '',
    search_text TEXT NOT NULL
);

CREATE INDEX idx_memory_beliefs_scope_status
    ON memory_beliefs(scope_kind, scope_ref, status, updated_at DESC);
CREATE INDEX idx_memory_beliefs_fingerprint
    ON memory_beliefs(fingerprint, scope_kind, scope_ref);
CREATE INDEX idx_memory_beliefs_status
    ON memory_beliefs(status, updated_at DESC);

CREATE VIRTUAL TABLE memory_beliefs_fts USING fts5(
    belief_id UNINDEXED,
    search_text,
    tokenize = 'unicode61'
);

CREATE TRIGGER memory_beliefs_fts_insert
AFTER INSERT ON memory_beliefs
BEGIN
    INSERT INTO memory_beliefs_fts(rowid, belief_id, search_text)
    VALUES (new.rowid, new.belief_id, new.search_text);
END;

CREATE TRIGGER memory_beliefs_fts_delete
AFTER DELETE ON memory_beliefs
BEGIN
    DELETE FROM memory_beliefs_fts WHERE rowid = old.rowid;
END;

CREATE TRIGGER memory_beliefs_fts_update
AFTER UPDATE OF search_text ON memory_beliefs
BEGIN
    DELETE FROM memory_beliefs_fts WHERE rowid = old.rowid;
    INSERT INTO memory_beliefs_fts(rowid, belief_id, search_text)
    VALUES (new.rowid, new.belief_id, new.search_text);
END;

CREATE TABLE memory_belief_evidence (
    belief_id TEXT NOT NULL,
    evidence_id TEXT NOT NULL,
    relation TEXT NOT NULL,
    PRIMARY KEY(belief_id, evidence_id, relation),
    FOREIGN KEY(belief_id) REFERENCES memory_beliefs(belief_id) ON DELETE CASCADE,
    FOREIGN KEY(evidence_id) REFERENCES memory_evidence(evidence_id) ON DELETE RESTRICT
);

CREATE INDEX idx_memory_belief_evidence_evidence
    ON memory_belief_evidence(evidence_id, belief_id, relation);

CREATE TABLE memory_belief_edges (
    from_belief_id TEXT NOT NULL,
    to_belief_id TEXT NOT NULL,
    relation TEXT NOT NULL,
    created_at TEXT NOT NULL,
    PRIMARY KEY(from_belief_id, to_belief_id, relation),
    FOREIGN KEY(from_belief_id) REFERENCES memory_beliefs(belief_id) ON DELETE CASCADE,
    FOREIGN KEY(to_belief_id) REFERENCES memory_beliefs(belief_id) ON DELETE CASCADE
);

CREATE INDEX idx_memory_belief_edges_from
    ON memory_belief_edges(from_belief_id, relation, to_belief_id);
CREATE INDEX idx_memory_belief_edges_to
    ON memory_belief_edges(to_belief_id, relation, from_belief_id);

CREATE TABLE memory_belief_status_history (
    seq INTEGER PRIMARY KEY AUTOINCREMENT,
    belief_id TEXT NOT NULL,
    from_status TEXT NOT NULL DEFAULT '',
    to_status TEXT NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    job_id TEXT NOT NULL DEFAULT '',
    changed_at TEXT NOT NULL,
    FOREIGN KEY(belief_id) REFERENCES memory_beliefs(belief_id) ON DELETE CASCADE
);

CREATE INDEX idx_memory_status_history_belief
    ON memory_belief_status_history(belief_id, seq);

CREATE TABLE memory_belief_uses (
    seq INTEGER PRIMARY KEY AUTOINCREMENT,
    belief_id TEXT NOT NULL,
    agent_id TEXT NOT NULL,
    invocation_id TEXT NOT NULL DEFAULT '',
    context_page_id TEXT NOT NULL DEFAULT '',
    purpose TEXT NOT NULL DEFAULT '',
    used_at TEXT NOT NULL,
    FOREIGN KEY(belief_id) REFERENCES memory_beliefs(belief_id) ON DELETE CASCADE
);

CREATE INDEX idx_memory_belief_uses_belief
    ON memory_belief_uses(belief_id, used_at DESC);

CREATE TABLE memory_propagation_jobs (
    job_id TEXT PRIMARY KEY,
    cause TEXT NOT NULL,
    state TEXT NOT NULL,
    max_depth INTEGER NOT NULL CHECK(max_depth > 0),
    max_work INTEGER NOT NULL CHECK(max_work > 0),
    processed_count INTEGER NOT NULL CHECK(processed_count >= 0),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    last_error TEXT NOT NULL DEFAULT ''
);

CREATE TABLE memory_propagation_queue (
    job_id TEXT NOT NULL,
    node_kind TEXT NOT NULL,
    node_id TEXT NOT NULL,
    depth INTEGER NOT NULL CHECK(depth >= 0),
    processed INTEGER NOT NULL CHECK(processed IN (0, 1)),
    enqueued_at TEXT NOT NULL,
    processed_at TEXT,
    PRIMARY KEY(job_id, node_kind, node_id),
    FOREIGN KEY(job_id) REFERENCES memory_propagation_jobs(job_id) ON DELETE CASCADE
);

CREATE INDEX idx_memory_propagation_pending
    ON memory_propagation_queue(job_id, processed, depth, enqueued_at, node_kind, node_id);
