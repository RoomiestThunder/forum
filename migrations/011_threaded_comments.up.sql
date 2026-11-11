-- parent_id makes a comment a reply; depth is stored so a reply's nesting level
-- is known without walking the tree. A comment that has replies is not removed
-- when deleted but kept as a tombstone (deleted = TRUE, content cleared), so the
-- replies stay attached to something.
ALTER TABLE comments ADD COLUMN IF NOT EXISTS parent_id INTEGER REFERENCES comments(id);
ALTER TABLE comments ADD COLUMN IF NOT EXISTS depth INTEGER NOT NULL DEFAULT 0;
ALTER TABLE comments ADD COLUMN IF NOT EXISTS deleted BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS comments_post_idx ON comments(post_id, created_at, id);
CREATE INDEX IF NOT EXISTS comments_parent_idx ON comments(parent_id);
