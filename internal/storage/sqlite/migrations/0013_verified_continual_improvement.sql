CREATE TABLE improvement_artifact_versions (
    artifact_id TEXT NOT NULL,
    version INTEGER NOT NULL CHECK (version > 0),
    kind TEXT NOT NULL,
    status TEXT NOT NULL,
    scope_level TEXT NOT NULL,
    scope_key TEXT NOT NULL DEFAULT '',
    payload_json TEXT NOT NULL,
    created_at TEXT NOT NULL,
    PRIMARY KEY (artifact_id, version)
);

CREATE INDEX improvement_artifact_scope_idx
ON improvement_artifact_versions (kind, scope_level, scope_key, status);

CREATE TABLE improvement_active_artifacts (
    artifact_id TEXT PRIMARY KEY,
    version INTEGER NOT NULL,
    FOREIGN KEY (artifact_id, version)
        REFERENCES improvement_artifact_versions (artifact_id, version)
);

CREATE TABLE improvement_evaluations (
    id TEXT PRIMARY KEY,
    artifact_id TEXT NOT NULL,
    baseline_version INTEGER NOT NULL,
    candidate_version INTEGER NOT NULL,
    payload_json TEXT NOT NULL,
    created_at TEXT NOT NULL,
    FOREIGN KEY (artifact_id, baseline_version)
        REFERENCES improvement_artifact_versions (artifact_id, version),
    FOREIGN KEY (artifact_id, candidate_version)
        REFERENCES improvement_artifact_versions (artifact_id, version)
);

CREATE TABLE improvement_promotions (
    artifact_id TEXT NOT NULL,
    candidate_version INTEGER NOT NULL,
    baseline_version INTEGER NOT NULL,
    evaluation_id TEXT NOT NULL,
    payload_json TEXT NOT NULL,
    promoted_at TEXT NOT NULL,
    PRIMARY KEY (artifact_id, candidate_version),
    FOREIGN KEY (artifact_id, candidate_version)
        REFERENCES improvement_artifact_versions (artifact_id, version)
);

CREATE TABLE improvement_rollbacks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    artifact_id TEXT NOT NULL,
    from_version INTEGER NOT NULL,
    to_version INTEGER NOT NULL,
    rolled_back_at TEXT NOT NULL,
    FOREIGN KEY (artifact_id, from_version)
        REFERENCES improvement_artifact_versions (artifact_id, version),
    FOREIGN KEY (artifact_id, to_version)
        REFERENCES improvement_artifact_versions (artifact_id, version)
);

CREATE TABLE improvement_invocation_manifests (
    invocation_id TEXT PRIMARY KEY,
    payload_json TEXT NOT NULL,
    captured_at TEXT NOT NULL
);

CREATE TABLE improvement_canary_usage (
    artifact_id TEXT NOT NULL,
    version INTEGER NOT NULL,
    used INTEGER NOT NULL DEFAULT 0 CHECK (used >= 0),
    PRIMARY KEY (artifact_id, version),
    FOREIGN KEY (artifact_id, version)
        REFERENCES improvement_artifact_versions (artifact_id, version)
);

CREATE TABLE improvement_runtime_control (
    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
    kill_switch_json TEXT NOT NULL
);

INSERT INTO improvement_runtime_control (singleton, kill_switch_json)
VALUES (1, '{"all":false,"families":{},"versions":{}}');
