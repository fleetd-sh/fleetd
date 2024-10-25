CREATE TABLE IF NOT EXISTS device (
    id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    name TEXT NOT NULL,
    type TEXT NOT NULL,
    status TEXT NOT NULL,
    last_seen TEXT NOT NULL
);