ALTER TABLE users ADD COLUMN IF NOT EXISTS is_admin BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE users ADD COLUMN IF NOT EXISTS is_moderator BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE users ADD COLUMN IF NOT EXISTS banned BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE posts ADD COLUMN IF NOT EXISTS pinned BOOLEAN NOT NULL DEFAULT FALSE;

CREATE TABLE IF NOT EXISTS reports (
    id          SERIAL PRIMARY KEY,
    reporter_id INTEGER NOT NULL REFERENCES users(id),
    target_type TEXT NOT NULL CHECK (target_type IN ('post', 'comment')),
    target_id   INTEGER NOT NULL,
    reason      TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'resolved')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS reports_status_idx ON reports(status, created_at DESC);
