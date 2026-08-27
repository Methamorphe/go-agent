CREATE TABLE ledger_events (
    ledger_sequence INTEGER PRIMARY KEY AUTOINCREMENT,
    event_id TEXT NOT NULL UNIQUE,
    agent_id TEXT NOT NULL,
    root_agent_id TEXT NOT NULL,

    process_version INTEGER NOT NULL
        CHECK (process_version > 0),

    event_type TEXT NOT NULL,

    schema_version INTEGER NOT NULL
        CHECK (schema_version >= 0),

    occurred_at TEXT NOT NULL,
    causation_id TEXT,
    correlation_id TEXT NOT NULL,

    actor_kind TEXT NOT NULL,
    actor_id TEXT NOT NULL DEFAULT '',

    payload_json BLOB NOT NULL
        CHECK (length(payload_json) <= 65536),

    UNIQUE (agent_id, process_version),

    FOREIGN KEY (causation_id)
        REFERENCES ledger_events(event_id)
);

CREATE INDEX idx_ledger_events_agent_version
    ON ledger_events(agent_id, process_version);

CREATE INDEX idx_ledger_events_root_sequence
    ON ledger_events(root_agent_id, ledger_sequence);

CREATE INDEX idx_ledger_events_correlation
    ON ledger_events(correlation_id, ledger_sequence);

CREATE INDEX idx_ledger_events_causation
    ON ledger_events(causation_id);


CREATE TABLE agent_processes (
    agent_id TEXT PRIMARY KEY,
    root_agent_id TEXT NOT NULL,
    parent_agent_id TEXT,

    creation_event_id TEXT NOT NULL UNIQUE,
    created_at TEXT NOT NULL,

    lineage_depth INTEGER NOT NULL
        CHECK (lineage_depth >= 0),

    version INTEGER NOT NULL
        CHECK (version > 0),

    status TEXT NOT NULL,

    state_schema_version INTEGER NOT NULL
        CHECK (state_schema_version > 0),

    state_json BLOB NOT NULL,
    updated_at TEXT NOT NULL,

    FOREIGN KEY (parent_agent_id)
        REFERENCES agent_processes(agent_id),

    FOREIGN KEY (creation_event_id)
        REFERENCES ledger_events(event_id)
);

CREATE INDEX idx_agent_processes_status
    ON agent_processes(
        status,
        updated_at,
        agent_id
    );

CREATE INDEX idx_agent_processes_root
    ON agent_processes(
        root_agent_id,
        lineage_depth,
        agent_id
    );


CREATE TABLE agent_snapshots (
    snapshot_id TEXT PRIMARY KEY,

    agent_id TEXT NOT NULL,

    through_process_version INTEGER NOT NULL
        CHECK (through_process_version > 0),

    through_ledger_sequence INTEGER NOT NULL
        CHECK (through_ledger_sequence > 0),

    schema_version INTEGER NOT NULL
        CHECK (schema_version > 0),

    created_at TEXT NOT NULL,

    state_json BLOB NOT NULL,
    sha256 TEXT NOT NULL,

    UNIQUE (
        agent_id,
        through_process_version
    ),

    FOREIGN KEY (agent_id)
        REFERENCES agent_processes(agent_id)
        ON DELETE CASCADE
);

CREATE INDEX idx_agent_snapshots_latest
    ON agent_snapshots(
        agent_id,
        through_process_version DESC
    );


CREATE TABLE command_receipts (
    request_id TEXT PRIMARY KEY,
    command_type TEXT NOT NULL,

    agent_id TEXT NOT NULL,

    result_version INTEGER NOT NULL
        CHECK (result_version > 0),

    result_json BLOB NOT NULL,

    last_event_id TEXT NOT NULL,
    created_at TEXT NOT NULL,

    FOREIGN KEY (agent_id)
        REFERENCES agent_processes(agent_id)
        ON DELETE CASCADE,

    FOREIGN KEY (last_event_id)
        REFERENCES ledger_events(event_id)
);


CREATE TABLE wake_conditions (
    agent_id TEXT PRIMARY KEY,

    sleep_id TEXT NOT NULL UNIQUE,
    wake_at TEXT NOT NULL,

    generation INTEGER NOT NULL
        CHECK (generation > 0),

    process_version INTEGER NOT NULL
        CHECK (process_version > 0),

    FOREIGN KEY (agent_id)
        REFERENCES agent_processes(agent_id)
        ON DELETE CASCADE
);

CREATE INDEX idx_wake_conditions_due
    ON wake_conditions(
        wake_at,
        agent_id
    );
