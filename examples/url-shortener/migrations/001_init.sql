-- Portable SQL schema for both PostgreSQL (production) and SQLite (local/tests).
CREATE TABLE links (
  code       TEXT PRIMARY KEY,
  target_url TEXT NOT NULL,
  clicks     INTEGER NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE clicks (
  id         TEXT PRIMARY KEY,
  link_code  TEXT NOT NULL REFERENCES links(code) ON DELETE CASCADE,
  user_agent TEXT NOT NULL DEFAULT '',
  referer    TEXT NOT NULL DEFAULT '',
  clicked_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_clicks_link_code ON clicks(link_code, clicked_at);
