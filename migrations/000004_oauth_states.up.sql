BEGIN;

CREATE TABLE IF NOT EXISTS oauth_states (
  state TEXT PRIMARY KEY,
  owner_id TEXT NOT NULL,
  expires_at BIGINT NOT NULL,
  created_at BIGINT NOT NULL DEFAULT (extract(epoch from now()))::bigint
);

ALTER TABLE oauth_states ENABLE ROW LEVEL SECURITY;
CREATE POLICY allow_authenticated_read_on_oauth_states
  ON oauth_states
  FOR SELECT
  USING (auth.uid() IS NOT NULL);

COMMIT;
