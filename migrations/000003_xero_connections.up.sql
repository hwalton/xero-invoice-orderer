BEGIN;

CREATE TABLE IF NOT EXISTS xero_connections (
  id TEXT PRIMARY KEY,                -- UUID or generated id
  owner_id TEXT NOT NULL UNIQUE,             -- your app's company/user id
  tenant_id TEXT NOT NULL,
  access_token TEXT NOT NULL,
  refresh_token TEXT NOT NULL,
  expires_at BIGINT,
  created_at BIGINT DEFAULT (extract(epoch from now()))::bigint,
  updated_at BIGINT DEFAULT (extract(epoch from now()))::bigint
);

ALTER TABLE xero_connections ENABLE ROW LEVEL SECURITY;
CREATE POLICY allow_authenticated_read_on_xero_connections
  ON xero_connections
  FOR SELECT
  USING (auth.uid() IS NOT NULL);

COMMIT;