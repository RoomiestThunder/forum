-- Renames made by the up migration are not reverted.
DROP INDEX IF EXISTS users_username_lower_idx;
