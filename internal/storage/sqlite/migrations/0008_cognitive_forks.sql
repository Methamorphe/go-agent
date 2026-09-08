CREATE TABLE cognitive_fork_checkpoints (
    checkpoint_id TEXT PRIMARY KEY,
    class TEXT NOT NULL,
    source_agent_id TEXT NOT NULL,
    root_agent_id TEXT NOT NULL,
    frontier_json BLOB NOT NULL,
    authority_json BLOB NOT NULL,
    last_event_id TEXT NOT NULL,
    process_state_json BLOB NOT NULL,
    state_hash TEXT NOT NULL,
    created_at TEXT NOT NULL,
    retain_until TEXT
);

CREATE INDEX idx_cognitive_fork_checkpoints_source
    ON cognitive_fork_checkpoints(source_agent_id, created_at);

CREATE TABLE cognitive_fork_groups (
    group_id TEXT PRIMARY KEY,
    checkpoint_id TEXT NOT NULL REFERENCES cognitive_fork_checkpoints(checkpoint_id) ON DELETE RESTRICT,
    source_agent_id TEXT NOT NULL,
    state TEXT NOT NULL,
    winner_fork_id TEXT,
    selection_reason TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX idx_cognitive_fork_groups_checkpoint
    ON cognitive_fork_groups(checkpoint_id);
CREATE INDEX idx_cognitive_fork_groups_state
    ON cognitive_fork_groups(state);

CREATE TABLE cognitive_fork_branches (
    fork_id TEXT PRIMARY KEY,
    group_id TEXT NOT NULL REFERENCES cognitive_fork_groups(group_id) ON DELETE CASCADE,
    agent_id TEXT NOT NULL,
    world_id TEXT NOT NULL,
    world_ref_json BLOB NOT NULL,
    authority_json BLOB NOT NULL,
    budget_limit_money_micros INTEGER NOT NULL CHECK (budget_limit_money_micros >= 0),
    budget_limit_tokens INTEGER NOT NULL CHECK (budget_limit_tokens >= 0),
    budget_spent_money_micros INTEGER NOT NULL DEFAULT 0 CHECK (budget_spent_money_micros >= 0),
    budget_spent_tokens INTEGER NOT NULL DEFAULT 0 CHECK (budget_spent_tokens >= 0),
    budget_reservation_id TEXT NOT NULL,
    budget_settled INTEGER NOT NULL DEFAULT 0 CHECK (budget_settled IN (0, 1)),
    state TEXT NOT NULL,
    evaluation_json BLOB,
    cognitive_overlay_json BLOB NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(group_id, agent_id),
    UNIQUE(group_id, world_id),
    UNIQUE(budget_reservation_id)
);

CREATE INDEX idx_cognitive_fork_branches_group
    ON cognitive_fork_branches(group_id);
CREATE INDEX idx_cognitive_fork_branches_state
    ON cognitive_fork_branches(state);
