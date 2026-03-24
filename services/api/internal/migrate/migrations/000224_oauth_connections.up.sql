CREATE TABLE oauth_connections (
  id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id        UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  provider       TEXT NOT NULL,
  provider_id    TEXT NOT NULL,
  provider_email TEXT,
  scopes         TEXT,
  access_token   BYTEA,
  refresh_token  BYTEA,
  token_expiry   TIMESTAMPTZ,
  created_at     TIMESTAMPTZ DEFAULT NOW(),
  updated_at     TIMESTAMPTZ DEFAULT NOW(),
  UNIQUE(provider, provider_id)
);
CREATE INDEX idx_oauth_connections_user ON oauth_connections(user_id);
CREATE INDEX idx_oauth_connections_provider_email ON oauth_connections(provider, provider_email);
