-- Accounts are looked up by email ignoring case (login, password reset, linking
-- a GitHub identity), which a plain index on email can't serve.
CREATE INDEX IF NOT EXISTS users_email_lower_idx ON users (LOWER(email));
