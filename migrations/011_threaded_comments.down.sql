DROP INDEX IF EXISTS comments_parent_idx;
DROP INDEX IF EXISTS comments_post_idx;
ALTER TABLE comments DROP COLUMN IF EXISTS deleted;
ALTER TABLE comments DROP COLUMN IF EXISTS depth;
ALTER TABLE comments DROP COLUMN IF EXISTS parent_id;
