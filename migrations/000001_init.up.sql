BEGIN;

CREATE OR REPLACE FUNCTION set_updated_at_epoch() RETURNS trigger AS $$
BEGIN
  NEW.updated_at := (extract(epoch from now()))::bigint;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TABLE IF NOT EXISTS parent_child (
    parent_id TEXT NOT NULL,
    child_id TEXT NOT NULL,
    quantity INTEGER DEFAULT 1,
    PRIMARY KEY (parent_id, child_id),
    created_at BIGINT DEFAULT (extract(epoch from now()))::bigint,
    updated_at BIGINT DEFAULT (extract(epoch from now()))::bigint
);

CREATE TABLE IF NOT EXISTS items_contacts (
    item_id TEXT NOT NULL,
    contact_id TEXT NOT NULL,
    PRIMARY KEY (item_id, contact_id),
    created_at BIGINT DEFAULT (extract(epoch from now()))::bigint,
    updated_at BIGINT DEFAULT (extract(epoch from now()))::bigint
);

CREATE TABLE IF NOT EXISTS shopping_list (
    list_id INTEGER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    item_id TEXT NOT NULL,
    quantity INTEGER NOT NULL DEFAULT 1,
    ordered BOOLEAN NOT NULL DEFAULT FALSE,
    created_at BIGINT DEFAULT (extract(epoch from now()))::bigint,
    updated_at BIGINT DEFAULT (extract(epoch from now()))::bigint
);

ALTER TABLE parent_child ENABLE ROW LEVEL SECURITY;
CREATE POLICY allow_authenticated_read_on_parent_child
  ON parent_child
  FOR SELECT
  USING (auth.uid() IS NOT NULL);

ALTER TABLE items_contacts ENABLE ROW LEVEL SECURITY;
CREATE POLICY allow_authenticated_read_on_items_contacts
  ON items_contacts
  FOR SELECT
  USING (auth.uid() IS NOT NULL);

ALTER TABLE shopping_list ENABLE ROW LEVEL SECURITY;
CREATE POLICY allow_authenticated_read_on_shopping_list
  ON shopping_list
  FOR SELECT
  USING (auth.uid() IS NOT NULL);

CREATE TRIGGER parent_child_set_updated_at
  BEFORE UPDATE ON parent_child
  FOR EACH ROW EXECUTE FUNCTION set_updated_at_epoch();

CREATE TRIGGER items_contacts_set_updated_at
  BEFORE UPDATE ON items_contacts
  FOR EACH ROW EXECUTE FUNCTION set_updated_at_epoch();

CREATE TRIGGER shopping_list_set_updated_at
  BEFORE UPDATE ON shopping_list
  FOR EACH ROW EXECUTE FUNCTION set_updated_at_epoch();

COMMIT;