CREATE TABLE agent_transactions (
    transaction_id TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL,
    base_checkpoint_id TEXT NOT NULL,
    world_id TEXT NOT NULL,
    branch_ref_json BLOB NOT NULL,
    state TEXT NOT NULL,
    version INTEGER NOT NULL CHECK (version >= 1),
    commit_policy TEXT NOT NULL,
    prepared_plan_json BLOB,
    reconcile_reason TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX idx_agent_transactions_state
    ON agent_transactions(state);

CREATE TABLE agent_transaction_effects (
    effect_id TEXT PRIMARY KEY,
    transaction_id TEXT NOT NULL REFERENCES agent_transactions(transaction_id) ON DELETE CASCADE,
    action_id TEXT NOT NULL,
    kind TEXT NOT NULL,
    effect_class TEXT NOT NULL,
    idempotent INTEGER NOT NULL CHECK (idempotent IN (0, 1)),
    retryable INTEGER NOT NULL CHECK (retryable IN (0, 1)),
    state TEXT NOT NULL,
    outcome_certainty TEXT NOT NULL,
    error TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(transaction_id, action_id)
);

CREATE INDEX idx_agent_transaction_effects_tx
    ON agent_transaction_effects(transaction_id);

CREATE INDEX idx_agent_transaction_effects_state
    ON agent_transaction_effects(state);

CREATE TABLE agent_transaction_verifications (
    verification_id TEXT PRIMARY KEY,
    transaction_id TEXT NOT NULL REFERENCES agent_transactions(transaction_id) ON DELETE CASCADE,
    status TEXT NOT NULL,
    verification_json BLOB NOT NULL,
    started_at TEXT NOT NULL,
    completed_at TEXT
);

CREATE INDEX idx_agent_transaction_verifications_tx
    ON agent_transaction_verifications(transaction_id);

CREATE TABLE agent_transaction_events (
    sequence INTEGER PRIMARY KEY AUTOINCREMENT,
    transaction_id TEXT NOT NULL REFERENCES agent_transactions(transaction_id) ON DELETE CASCADE,
    transaction_version INTEGER NOT NULL CHECK (transaction_version >= 1),
    event_type TEXT NOT NULL,
    payload_json BLOB,
    created_at TEXT NOT NULL
);

CREATE INDEX idx_agent_transaction_events_tx_sequence
    ON agent_transaction_events(transaction_id, sequence);
