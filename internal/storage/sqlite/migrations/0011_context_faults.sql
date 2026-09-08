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

CREATE INDEX IF NOT EXISTS idx_context_faults_agent_pending
    ON context_faults(agent_id, state, manifested_invocation_id, updated_at);

CREATE INDEX IF NOT EXISTS idx_context_faults_source_invocation
    ON context_faults(source_invocation_id, updated_at);
