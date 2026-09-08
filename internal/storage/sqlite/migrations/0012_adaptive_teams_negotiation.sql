CREATE TABLE adaptive_teams (
    team_id TEXT PRIMARY KEY,
    state TEXT NOT NULL,
    team_json TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX adaptive_teams_state_updated_idx
    ON adaptive_teams(state, updated_at);

CREATE TABLE agent_negotiations (
    negotiation_id TEXT PRIMARY KEY,
    team_id TEXT NOT NULL,
    state TEXT NOT NULL,
    version INTEGER NOT NULL,
    round INTEGER NOT NULL,
    negotiation_json TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY (team_id) REFERENCES adaptive_teams(team_id) ON DELETE CASCADE
);

CREATE INDEX agent_negotiations_team_state_idx
    ON agent_negotiations(team_id, state, updated_at);

CREATE TABLE agent_negotiation_turns (
    negotiation_id TEXT NOT NULL,
    sequence INTEGER NOT NULL,
    round INTEGER NOT NULL,
    actor_agent_id TEXT NOT NULL,
    kind TEXT NOT NULL,
    turn_json TEXT NOT NULL,
    created_at TEXT NOT NULL,
    PRIMARY KEY (negotiation_id, sequence),
    FOREIGN KEY (negotiation_id) REFERENCES agent_negotiations(negotiation_id) ON DELETE CASCADE
);

CREATE INDEX agent_negotiation_turns_actor_idx
    ON agent_negotiation_turns(negotiation_id, actor_agent_id, sequence);
