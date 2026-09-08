CREATE UNIQUE INDEX idx_memory_beliefs_fork_promotion
    ON memory_beliefs(origin_fork_id, fingerprint, scope_kind, scope_ref)
    WHERE origin_fork_id <> '';
