CREATE TABLE IF NOT EXISTS tags (
    id   SERIAL PRIMARY KEY,
    name TEXT UNIQUE NOT NULL
);

CREATE TABLE IF NOT EXISTS post_tags (
    post_id INTEGER REFERENCES posts(id),
    tag_id  INTEGER REFERENCES tags(id)
);

CREATE TABLE IF NOT EXISTS bookmarks (
    id         SERIAL PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users(id),
    post_id    INTEGER NOT NULL REFERENCES posts(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, post_id)
);
