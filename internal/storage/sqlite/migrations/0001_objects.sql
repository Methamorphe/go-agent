CREATE TABLE IF NOT EXISTS objects (
    ref TEXT PRIMARY KEY,
    algorithm TEXT NOT NULL,
    digest TEXT NOT NULL,
    size_bytes INTEGER NOT NULL CHECK (size_bytes >= 0),
    media_type TEXT NOT NULL DEFAULT '',
    compression TEXT NOT NULL DEFAULT 'none',
    created_at TEXT NOT NULL,
    UNIQUE (algorithm, digest)
)
