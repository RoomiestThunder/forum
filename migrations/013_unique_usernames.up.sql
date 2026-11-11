-- username was never unique, although profiles, direct messages, mentions and
-- blocks all resolve a user by name - so two accounts could share one and
-- impersonate each other. Duplicates that already exist must be renamed before
-- the index can be built: the oldest account keeps the name and each later one
-- gets its id appended.
UPDATE users SET username = username || '_' || id
WHERE EXISTS (
    SELECT 1 FROM users o
    WHERE LOWER(o.username) = LOWER(users.username) AND o.id < users.id
);

CREATE UNIQUE INDEX IF NOT EXISTS users_username_lower_idx ON users (LOWER(username));
