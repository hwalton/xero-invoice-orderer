CREATE TABLE IF NOT EXISTS xero_connections (
  id TEXT PRIMARY KEY,                -- UUID or generated id
  owner_id TEXT NOT NULL,             -- your app's company/user id
  tenant_id TEXT NOT NULL,
  access_token TEXT NOT NULL,
  refresh_token TEXT NOT NULL,
  expires_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ DEFAULT now(),
  updated_at TIMESTAMPTZ DEFAULT now()
);