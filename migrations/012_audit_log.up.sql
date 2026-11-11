-- Who did what to whom, for actions taken by staff. details is a JSON object
-- snapshotting what was acted on (a deleted post's title, a deleted comment's
-- text), since after a delete the row it described no longer exists.
CREATE TABLE IF NOT EXISTS audit_log (
    id          SERIAL PRIMARY KEY,
    actor_id    INTEGER NOT NULL REFERENCES users(id),
    action      TEXT NOT NULL,
    target_type TEXT NOT NULL,
    target_id   INTEGER NOT NULL,
    details     TEXT NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS audit_log_created_idx ON audit_log(created_at DESC, id DESC);
