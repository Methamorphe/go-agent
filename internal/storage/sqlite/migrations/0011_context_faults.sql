CREATE TABLE IF NOT EXISTS context_faults (
    fault_id TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL,
    source_invocation_id TEXT NOT NULL,
    state TEXT NOT NULL,
    record_json TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    manifested_invocation_id TEXT
);

CREATE TABLE IF NOT EXISTS context_fault_events (
    sequence INTEGER PRIMARY KEY AUTOINCREMENT,
    fault_id TEXT NOT NULL,
    state TEXT NOT NULL,
    record_json TEXT NOT NULL,
    recorded_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_context_faults_agent_pending
    ON context_faults(agent_id, state, manifested_invocation_id, updated_at);

CREATE INDEX IF NOT EXISTS idx_context_faults_source_invocation
    ON context_faults(source_invocation_id, updated_at);

CREATE INDEX IF NOT EXISTS idx_context_fault_events_fault
    ON context_fault_events(fault_id, sequence);

CREATE TRIGGER IF NOT EXISTS context_manifest_claim_resolved_faults
AFTER INSERT ON context_manifests
BEGIN
    UPDATE context_faults
    SET manifested_invocation_id = new.invocation_id
    WHERE agent_id = new.agent_id
      AND state = 'RESOLVED'
      AND manifested_invocation_id IS NULL;
END;
