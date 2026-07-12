CREATE TABLE IF NOT EXISTS sessions (
    id      SERIAL PRIMARY KEY,
    user_id INTEGER REFERENCES users(id),
    uuid    TEXT UNIQUE,
    expires TIMESTAMPTZ
);
