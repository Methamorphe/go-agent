CREATE TABLE scheduler_budget_accounts (
    root_agent_id TEXT PRIMARY KEY,
    limit_money_micros INTEGER NOT NULL CHECK (limit_money_micros >= 0),
    limit_tokens INTEGER NOT NULL CHECK (limit_tokens >= 0),
    spent_money_micros INTEGER NOT NULL DEFAULT 0 CHECK (spent_money_micros >= 0),
    spent_tokens INTEGER NOT NULL DEFAULT 0 CHECK (spent_tokens >= 0),
    reserved_money_micros INTEGER NOT NULL DEFAULT 0 CHECK (reserved_money_micros >= 0),
    reserved_tokens INTEGER NOT NULL DEFAULT 0 CHECK (reserved_tokens >= 0)
);

CREATE TABLE scheduler_budget_reservations (
    reservation_id TEXT PRIMARY KEY,
    root_agent_id TEXT NOT NULL REFERENCES scheduler_budget_accounts(root_agent_id) ON DELETE CASCADE,
    amount_money_micros INTEGER NOT NULL CHECK (amount_money_micros >= 0),
    amount_tokens INTEGER NOT NULL CHECK (amount_tokens >= 0),
    settled INTEGER NOT NULL DEFAULT 0 CHECK (settled IN (0, 1))
);

CREATE INDEX idx_scheduler_budget_reservations_root
    ON scheduler_budget_reservations(root_agent_id);
