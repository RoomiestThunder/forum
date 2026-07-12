CREATE TABLE IF NOT EXISTS users (
    id           SERIAL PRIMARY KEY,
    email        TEXT UNIQUE NOT NULL,
    username     TEXT NOT NULL,
    password     TEXT NOT NULL,
    avatar_url   TEXT NOT NULL DEFAULT '',
    bio          TEXT NOT NULL DEFAULT '',
    is_admin     BOOLEAN NOT NULL DEFAULT FALSE,
    is_moderator BOOLEAN NOT NULL DEFAULT FALSE,
    banned       BOOLEAN NOT NULL DEFAULT FALSE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS categories (
    id   SERIAL PRIMARY KEY,
    name TEXT UNIQUE NOT NULL
);

INSERT INTO categories (name) VALUES ('General'), ('Programming'), ('Offtopic')
    ON CONFLICT DO NOTHING;

CREATE TABLE IF NOT EXISTS posts (
    id           SERIAL PRIMARY KEY,
    user_id      INTEGER REFERENCES users(id),
    title        TEXT,
    content      TEXT,
    image_url    TEXT NOT NULL DEFAULT '',
    pinned       BOOLEAN NOT NULL DEFAULT FALSE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    search_vector tsvector GENERATED ALWAYS AS (
        setweight(to_tsvector('english', coalesce(title, '')), 'A') ||
        setweight(to_tsvector('english', coalesce(content, '')), 'B')
    ) STORED
);

CREATE INDEX IF NOT EXISTS posts_search_idx ON posts USING GIN(search_vector);

CREATE TABLE IF NOT EXISTS post_categories (
    post_id     INTEGER REFERENCES posts(id),
    category_id INTEGER REFERENCES categories(id)
);

CREATE TABLE IF NOT EXISTS comments (
    id         SERIAL PRIMARY KEY,
    post_id    INTEGER REFERENCES posts(id),
    user_id    INTEGER REFERENCES users(id),
    content    TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS post_likes (
    id      SERIAL PRIMARY KEY,
    post_id INTEGER REFERENCES posts(id),
    user_id INTEGER REFERENCES users(id),
    is_like BOOLEAN,
    UNIQUE (post_id, user_id)
);

CREATE TABLE IF NOT EXISTS comment_likes (
    id         SERIAL PRIMARY KEY,
    comment_id INTEGER REFERENCES comments(id),
    user_id    INTEGER REFERENCES users(id),
    is_like    BOOLEAN,
    UNIQUE (comment_id, user_id)
);

CREATE TABLE IF NOT EXISTS sessions (
    id      SERIAL PRIMARY KEY,
    user_id INTEGER REFERENCES users(id),
    uuid    TEXT UNIQUE,
    expires TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS refresh_tokens (
    id         SERIAL PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users(id),
    token      TEXT UNIQUE NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS uploads (
    id           SERIAL PRIMARY KEY,
    user_id      INTEGER NOT NULL REFERENCES users(id),
    filename     TEXT NOT NULL,
    object_key   TEXT NOT NULL,
    content_type TEXT NOT NULL,
    size         BIGINT NOT NULL,
    url          TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

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

CREATE TABLE IF NOT EXISTS notifications (
    id         SERIAL PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users(id),
    type       TEXT NOT NULL,
    payload    TEXT NOT NULL,
    read       BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS notifications_user_idx ON notifications(user_id, created_at DESC);

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
