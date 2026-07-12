-- Cookie-based sessions were replaced by JWT access/refresh tokens once the
-- SPA became the only frontend, so the sessions table is no longer read or
-- written by anything.
DROP TABLE IF EXISTS sessions;
