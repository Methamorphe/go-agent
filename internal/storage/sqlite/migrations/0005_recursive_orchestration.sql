CREATE TABLE orchestration_accounts (
    agent_id TEXT PRIMARY KEY,
    root_id TEXT NOT NULL,
    authority_json TEXT NOT NULL,
    available_json TEXT NOT NULL,
    reserved_json TEXT NOT NULL,
    policy_json TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX orchestration_accounts_root_idx ON orchestration_accounts(root_id);

CREATE TABLE orchestration_spawns (
    spawn_id TEXT PRIMARY KEY,
    parent_agent_id TEXT NOT NULL,
    child_agent_id TEXT,
    root_agent_id TEXT NOT NULL,
    depth INTEGER NOT NULL,
    task_intent TEXT NOT NULL,
    task_key TEXT NOT NULL,
    authority_json TEXT NOT NULL,
    reservation_id TEXT NOT NULL UNIQUE,
    reserved_json TEXT NOT NULL,
    status TEXT NOT NULL,
    reject_reason TEXT,
    actual_json TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX orchestration_spawns_parent_idx ON orchestration_spawns(parent_agent_id, status);
CREATE INDEX orchestration_spawns_root_idx ON orchestration_spawns(root_agent_id, status);
CREATE INDEX orchestration_spawns_reuse_idx ON orchestration_spawns(root_agent_id, task_key, status);

CREATE TABLE orchestration_messages (
    message_id TEXT PRIMARY KEY,
    from_agent_id TEXT NOT NULL,
    to_agent_id TEXT NOT NULL,
    message_type TEXT NOT NULL,
    payload_ref TEXT,
    inline_payload TEXT,
    correlation_id TEXT,
    payload_bytes INTEGER NOT NULL,
    created_at TEXT NOT NULL,
    consumed_at TEXT
);
CREATE INDEX orchestration_messages_mailbox_idx ON orchestration_messages(to_agent_id, consumed_at, created_at);

CREATE TABLE orchestration_waits (
    wait_id TEXT PRIMARY KEY,
    parent_agent_id TEXT NOT NULL UNIQUE,
    mode TEXT NOT NULL,
    quorum INTEGER NOT NULL,
    failure_policy TEXT NOT NULL,
    children_json TEXT NOT NULL,
    created_at TEXT NOT NULL
);
CREATE TABLE orchestration_wait_edges (
    wait_id TEXT NOT NULL,
    from_agent_id TEXT NOT NULL,
    to_agent_id TEXT NOT NULL,
    PRIMARY KEY(wait_id, to_agent_id),
    FOREIGN KEY(wait_id) REFERENCES orchestration_waits(wait_id) ON DELETE CASCADE
);
CREATE INDEX orchestration_wait_edges_from_idx ON orchestration_wait_edges(from_agent_id);

CREATE TABLE orchestration_results (
    child_agent_id TEXT PRIMARY KEY,
    result_json TEXT NOT NULL,
    completed_at TEXT NOT NULL
);

CREATE TABLE orchestration_admission (
    agent_id TEXT PRIMARY KEY,
    root_id TEXT NOT NULL,
    priority INTEGER NOT NULL,
    enqueued_at TEXT NOT NULL,
    active INTEGER NOT NULL DEFAULT 0,
    admitted_at TEXT
);
CREATE INDEX orchestration_admission_queue_idx ON orchestration_admission(active, priority DESC, enqueued_at, root_id);
