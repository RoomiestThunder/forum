CREATE TABLE IF NOT EXISTS users (
    id             SERIAL PRIMARY KEY,
    email          TEXT UNIQUE NOT NULL,
    username       TEXT NOT NULL,
    password       TEXT NOT NULL,
    avatar_url     TEXT NOT NULL DEFAULT '',
    bio            TEXT NOT NULL DEFAULT '',
    is_admin       BOOLEAN NOT NULL DEFAULT FALSE,
    is_moderator   BOOLEAN NOT NULL DEFAULT FALSE,
    banned         BOOLEAN NOT NULL DEFAULT FALSE,
    email_verified BOOLEAN NOT NULL DEFAULT FALSE,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS users_username_lower_idx ON users (LOWER(username));
CREATE INDEX IF NOT EXISTS users_email_lower_idx ON users (LOWER(email));

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
    draft        BOOLEAN NOT NULL DEFAULT FALSE,
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
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    parent_id  INTEGER REFERENCES comments(id),
    depth      INTEGER NOT NULL DEFAULT 0,
    deleted    BOOLEAN NOT NULL DEFAULT FALSE
);
CREATE INDEX IF NOT EXISTS comments_post_idx ON comments(post_id, created_at, id);
CREATE INDEX IF NOT EXISTS comments_parent_idx ON comments(parent_id);

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

CREATE TABLE IF NOT EXISTS email_verification_tokens (
    id         SERIAL PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users(id),
    token      TEXT UNIQUE NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS password_reset_tokens (
    id         SERIAL PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users(id),
    token      TEXT UNIQUE NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    used       BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS oauth_identities (
    id               SERIAL PRIMARY KEY,
    user_id          INTEGER NOT NULL REFERENCES users(id),
    provider         TEXT NOT NULL,
    provider_user_id TEXT NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (provider, provider_user_id)
);

CREATE TABLE IF NOT EXISTS conversations (
    id              SERIAL PRIMARY KEY,
    user_a_id       INTEGER NOT NULL REFERENCES users(id),
    user_b_id       INTEGER NOT NULL REFERENCES users(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_message_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_a_id, user_b_id)
);

CREATE TABLE IF NOT EXISTS messages (
    id              SERIAL PRIMARY KEY,
    conversation_id INTEGER NOT NULL REFERENCES conversations(id),
    sender_id       INTEGER NOT NULL REFERENCES users(id),
    content         TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    read_at         TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_messages_conversation ON messages(conversation_id);

CREATE TABLE IF NOT EXISTS user_blocks (
    blocker_id INTEGER NOT NULL REFERENCES users(id),
    blocked_id INTEGER NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (blocker_id, blocked_id),
    CHECK (blocker_id <> blocked_id)
);

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
